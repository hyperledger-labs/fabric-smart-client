/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"os"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var commLogger = logging.MustGetLogger()

type TLSOption func(tlsConfig *tls.Config)

func ServerNameOverride(name string) TLSOption {
	return func(tlsConfig *tls.Config) {
		tlsConfig.ServerName = name
	}
}

func CertPoolOverride(pool *x509.CertPool) TLSOption {
	return func(tlsConfig *tls.Config) {
		tlsConfig.RootCAs = pool
	}
}

// GetTLSCertHash computes SHA2-256 on tls certificate
func GetTLSCertHash(cert *tls.Certificate) ([]byte, error) {
	if cert == nil || len(cert.Certificate) == 0 {
		return nil, nil
	}

	tlsCertHash := sha256.Sum256(cert.Certificate[0])
	return tlsCertHash[:], nil
}

// Client models a GRPC client
type Client struct {
	// TLS configuration used by the grpc.ClientConn
	tlsConfig *tls.Config
	// Options for setting up new connections
	dialOpts []grpc.DialOption
	// Duration for which to block while established a new connection
	timeout time.Duration
	// Maximum message size the client can receive
	maxRecvMsgSize int
	// Maximum message size the client can send
	maxSendMsgSize int
	// TODO: improve by providing grpc connection pool
	// Opened GRPC client connections to be closed
	grpcConns []*grpc.ClientConn
	// Mutex on grpcConns
	grpcCMux sync.Mutex
}

// NewGRPCClient creates a new implementation of Client given an address
// and client configuration
func NewGRPCClient(config ClientConfig) (*Client, error) {
	client := &Client{
		grpcConns: []*grpc.ClientConn{},
	}

	// parse secure options
	err := client.parseSecureOptions(config.SecOpts)
	if err != nil {
		return client, err
	}

	client.dialOpts = append(client.dialOpts, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	// set keepalive
	client.dialOpts = append(client.dialOpts, ClientKeepaliveOptions(config.KeepAliveConfig)...)
	// Unless asynchronous connect is set, make connection establishment blocking.
	if !config.AsyncConnect {
		//lint:ignore SA1019 Refactor in next change
		client.dialOpts = append(client.dialOpts, grpc.WithBlock()) //nolint:all
		//lint:ignore SA1019 Refactor in next change
		client.dialOpts = append(client.dialOpts, grpc.FailOnNonTempDialError(true)) //nolint:all
	}
	client.timeout = config.Timeout
	// set send/recv message size to package defaults
	client.maxRecvMsgSize = MaxRecvMsgSize
	client.maxSendMsgSize = MaxSendMsgSize

	return client, nil
}

type TLSClientConfig struct {
	TLSClientAuthRequired bool
	TLSClientKeyFile      string
	TLSClientCertFile     string
}

func CreateSecOpts(connConfig ConnectionConfig, cliConfig TLSClientConfig) (*SecureOptions, error) {
	return createSecOpts(connConfig, false, &cliConfig)
}

func createTLSSecOpts(connConfig ConnectionConfig) (*SecureOptions, error) {
	return createSecOpts(connConfig, true, nil)
}

func createSecOpts(connConfig ConnectionConfig, forceTLS bool, cliConfig *TLSClientConfig) (*SecureOptions, error) {
	var certs [][]byte
	if connConfig.TLSEnabled {
		switch {
		case len(connConfig.TLSRootCertFile) != 0:
			caPEM, err := os.ReadFile(connConfig.TLSRootCertFile)
			if err != nil {
				return nil, errors.WithMessagef(err, "unable to load TLS cert from %s", connConfig.TLSRootCertFile)
			}
			certs = append(certs, caPEM)
		case len(connConfig.TLSRootCertBytes) != 0:
			certs = connConfig.TLSRootCertBytes
		default:
			return nil, errors.New("missing TLSRootCertFile in client config")
		}
	}

	tlsEnabled := connConfig.TLSEnabled || forceTLS
	secOpts := &SecureOptions{
		UseTLS:            tlsEnabled,
		RequireClientCert: !tlsEnabled && cliConfig.TLSClientAuthRequired,
	}

	if secOpts.RequireClientCert {
		keyPEM, err := os.ReadFile(cliConfig.TLSClientKeyFile)
		if err != nil {
			return nil, errors.WithMessage(err, "unable to load fabric.tls.clientKey.file")
		}
		secOpts.Key = keyPEM
		certPEM, err := os.ReadFile(cliConfig.TLSClientCertFile)
		if err != nil {
			return nil, errors.WithMessage(err, "unable to load fabric.tls.clientCert.file")
		}
		secOpts.Certificate = certPEM
	}

	if tlsEnabled {
		if len(certs) == 0 {
			return nil, errors.New("tls root cert file must be set")
		}
		secOpts.ServerRootCAs = certs
	}
	return secOpts, nil
}

// CreateGRPCClient returns a comm.Client based on toke client config
func CreateGRPCClient(config *ConnectionConfig) (*Client, error) {
	secOpts, err := createTLSSecOpts(*config)
	if err != nil {
		return nil, err
	}
	timeout := config.ConnectionTimeout
	if timeout <= 0 {
		timeout = DefaultConnectionTimeout
	}
	return NewGRPCClient(ClientConfig{
		Timeout: timeout,
		SecOpts: *secOpts,
	})
}

func (client *Client) parseSecureOptions(opts SecureOptions) error {
	// if TLS is not enabled, return
	if !opts.UseTLS {
		return nil
	}

	client.tlsConfig = &tls.Config{
		VerifyPeerCertificate: opts.VerifyCertificate,
		MinVersion:            tls.VersionTLS12} // TLS 1.2 only
	if len(opts.ServerRootCAs) > 0 {
		client.tlsConfig.RootCAs = x509.NewCertPool()
		for _, certBytes := range opts.ServerRootCAs {
			err := AddPemToCertPool(certBytes, client.tlsConfig.RootCAs)
			if err != nil {
				commLogger.Errorf("error adding root certificate: %v", err)
				return errors.WithMessage(err,
					"error adding root certificate")
			}
		}
	}
	if opts.RequireClientCert {
		// make sure we have both Key and Certificate
		if opts.Key != nil &&
			opts.Certificate != nil {
			cert, err := tls.X509KeyPair(opts.Certificate,
				opts.Key)
			if err != nil {
				return errors.WithMessage(err, "failed to "+
					"load client certificate")
			}
			client.tlsConfig.Certificates = append(
				client.tlsConfig.Certificates, cert)
		} else {
			return errors.New("both Key and Certificate " +
				"are required when using mutual TLS")
		}
	}

	if opts.TimeShift > 0 {
		client.tlsConfig.Time = func() time.Time {
			return time.Now().Add((-1) * opts.TimeShift)
		}
	}

	return nil
}

// Certificate returns the tls.Certificate used to make TLS connections
// when client certificates are required by the server
func (client *Client) Certificate() tls.Certificate {
	cert := tls.Certificate{}
	if client.tlsConfig != nil && len(client.tlsConfig.Certificates) > 0 {
		cert = client.tlsConfig.Certificates[0]
	}
	return cert
}

// TLSEnabled is a flag indicating whether to use TLS for client
// connections
func (client *Client) TLSEnabled() bool {
	return client.tlsConfig != nil
}

// MutualTLSRequired is a flag indicating whether the client
// must send a certificate when making TLS connections
func (client *Client) MutualTLSRequired() bool {
	return client.tlsConfig != nil &&
		len(client.tlsConfig.Certificates) > 0
}

// SetMaxRecvMsgSize sets the maximum message size the client can receive
func (client *Client) SetMaxRecvMsgSize(size int) {
	client.maxRecvMsgSize = size
}

// SetMaxSendMsgSize sets the maximum message size the client can send
func (client *Client) SetMaxSendMsgSize(size int) {
	client.maxSendMsgSize = size
}

// SetServerRootCAs sets the list of authorities used to verify server
// certificates based on a list of PEM-encoded X509 certificate authorities
func (client *Client) SetServerRootCAs(serverRoots [][]byte) error {

	// NOTE: if no serverRoots are specified, the current cert pool will be
	// replaced with an empty one
	certPool := x509.NewCertPool()
	for _, root := range serverRoots {
		err := AddPemToCertPool(root, certPool)
		if err != nil {
			return errors.WithMessage(err, "error adding root certificate")
		}
	}
	client.tlsConfig.RootCAs = certPool
	return nil
}

// NewConnection returns a grpc.ClientConn for the target address and
// overrides the server name used to verify the hostname on the
// certificate returned by a server when using TLS
func (client *Client) NewConnection(address string, tlsOptions ...TLSOption) (*grpc.ClientConn, error) {
	if len(address) == 0 {
		return nil, errors.New("address is empty")
	}

	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, client.dialOpts...)

	// set transport credentials and max send/recv message sizes
	// immediately before creating a connection in order to allow
	// SetServerRootCAs / SetMaxRecvMsgSize / SetMaxSendMsgSize
	//  to take effect on a per connection basis
	if client.tlsConfig != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(
			&DynamicClientCredentials{
				TLSConfig:  client.tlsConfig,
				TLSOptions: tlsOptions,
			},
		))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	dialOpts = append(dialOpts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(client.maxRecvMsgSize),
		grpc.MaxCallSendMsgSize(client.maxSendMsgSize),
	))

	ctx, cancel := context.WithTimeout(context.Background(), client.timeout)
	defer cancel()
	//lint:ignore SA1019 Refactor in next change
	conn, err := grpc.DialContext(ctx, address, dialOpts...) //nolint:all
	if err != nil {
		commLogger.Debugf("failed to create new connection to [%s][%v]: [%s]", address, dialOpts, errors.WithStack(err))
		return nil, errors.WithMessage(errors.WithStack(err), "failed to create new connection")
	}

	client.grpcCMux.Lock()
	client.grpcConns = append(client.grpcConns, conn)
	client.grpcCMux.Unlock()
	return conn, nil
}

func (client *Client) Close() {
	commLogger.Debugf("closing %d grpc connections", len(client.grpcConns))
	for _, grpcCon := range client.grpcConns {
		if err := grpcCon.Close(); err != nil {
			commLogger.Warningf("unable to close grpc conn but continue. Reason: %s", err.Error())
		}
	}
}
