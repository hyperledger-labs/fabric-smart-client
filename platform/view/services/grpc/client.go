/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"sync"
	"time"

	"google.golang.org/grpc/credentials/insecure"

	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

var commLogger = flogging.MustGetLogger("view-sdk.comm")

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

// Hasher is the interface provides the hash function should be used for all token components.
type Hasher interface {
	Hash(msg []byte) (hash []byte, err error)
}

// GetTLSCertHash computes SHA2-256 on tls certificate
func GetTLSCertHash(cert *tls.Certificate, hasher Hasher) ([]byte, error) {
	if cert == nil || len(cert.Certificate) == 0 {
		return nil, nil
	}

	tlsCertHash, err := hasher.Hash(cert.Certificate[0])
	if err != nil {
		return nil, errors.WithMessage(err, "failed to compute SHA256 on client certificate")
	}
	return tlsCertHash, nil
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

	// keepalive options

	kap := keepalive.ClientParameters{
		Time:                config.KaOpts.ClientInterval,
		Timeout:             config.KaOpts.ClientTimeout,
		PermitWithoutStream: true,
	}
	// set keepalive
	client.dialOpts = append(client.dialOpts, grpc.WithKeepaliveParams(kap))
	// Unless asynchronous connect is set, make connection establishment blocking.
	if !config.AsyncConnect {
		client.dialOpts = append(client.dialOpts, grpc.WithBlock())
		client.dialOpts = append(client.dialOpts, grpc.FailOnNonTempDialError(true))
	}
	client.timeout = config.Timeout
	// set send/recv message size to package defaults
	client.maxRecvMsgSize = MaxRecvMsgSize
	client.maxSendMsgSize = MaxSendMsgSize

	return client, nil
}

// CreateGRPCClient returns a comm.Client based on toke client config
func CreateGRPCClient(config *ConnectionConfig) (*Client, error) {
	timeout := config.ConnectionTimeout
	if timeout <= 0 {
		timeout = DefaultConnectionTimeout
	}
	clientConfig := ClientConfig{
		KaOpts: KeepaliveOptions{
			ClientInterval:    60 * time.Second,
			ClientTimeout:     60 * time.Second,
			ServerInterval:    30 * time.Second,
			ServerTimeout:     60 * time.Second,
			ServerMinInterval: 60 * time.Second,
		},
		Timeout: timeout,
	}

	if config.TLSEnabled {
		var certs [][]byte
		switch {
		case len(config.TLSRootCertFile) != 0:
			caPEM, err := ioutil.ReadFile(config.TLSRootCertFile)
			if err != nil {
				return nil, errors.WithMessagef(err, "unable to load TLS cert from %s", config.TLSRootCertFile)
			}
			certs = append(certs, caPEM)
		case len(config.TLSRootCertBytes) != 0:
			certs = config.TLSRootCertBytes
		default:
			return nil, errors.New("missing TLSRootCertFile in client config")
		}

		secOpts := SecureOptions{
			UseTLS:            true,
			ServerRootCAs:     certs,
			RequireClientCert: false,
		}
		clientConfig.SecOpts = secOpts
	}

	return NewGRPCClient(clientConfig)
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
	conn, err := grpc.DialContext(ctx, address, dialOpts...)
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
	if commLogger.IsEnabledFor(zapcore.DebugLevel) {
		commLogger.Debugf("closing %d grpc connections", len(client.grpcConns))
	}
	for _, grpcCon := range client.grpcConns {
		if err := grpcCon.Close(); err != nil {
			commLogger.Warningf("unable to close grpc conn but continue. Reason: %s", err.Error())
		}
	}
}
