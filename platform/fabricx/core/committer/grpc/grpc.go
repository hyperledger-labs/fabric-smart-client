/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// ErrInvalidAddress is returned when an endpoint address is empty.
var ErrInvalidAddress = errors.New("empty address")

// ServiceConfigProvider provides gRPC configuration for a given network.
//
//go:generate counterfeiter -o mock/service_config_provider.go --fake-name ServiceConfigProvider . ServiceConfigProvider
type ServiceConfigProvider interface {
	// NotificationServiceConfig returns the configuration for the notification service for the specified network.
	NotificationServiceConfig(network string) (*config.Config, error)
	// QueryServiceConfig returns the configuration for the query service for the specified network.
	QueryServiceConfig(network string) (*config.Config, error)
}

// ClientProvider provides gRPC client connections for a given network.
//
// Connections are cached per (service, network) to prevent goroutine groth.
type ClientProvider struct {
	// configProvider is used to retrieve the configuration for a network.
	configProvider ServiceConfigProvider

	notificationConn sync.Map // network -> *grpc.ClientConn
	queryConn        sync.Map // network -> *grpc.ClientConn
}

// NewClientProvider returns a new ClientProvider instance.
func NewClientProvider(configProvider ServiceConfigProvider) *ClientProvider {
	return &ClientProvider{configProvider: configProvider}
}

// NotificationServiceClient returns a gRPC client connection to the notification service for the specified network.
// The connection is created on first use and cached for subsequent calls.
func (c *ClientProvider) NotificationServiceClient(network string) (*grpc.ClientConn, error) {
	return c.getOrCreate(&c.notificationConn, network, c.configProvider.NotificationServiceConfig)
}

// QueryServiceClient returns a gRPC client connection to the query service for the specified network.
// The connection is created on first use and cached for subsequent calls.
func (c *ClientProvider) QueryServiceClient(network string) (*grpc.ClientConn, error) {
	return c.getOrCreate(&c.queryConn, network, c.configProvider.QueryServiceConfig)
}

// getOrCreate returns the cached *grpc.ClientConn for the given network, or
// dials a new one via loadCfg and caches it. Under a benign race two callers
// may both dial; the loser closes its connection and returns the winner's.
func (c *ClientProvider) getOrCreate(
	cache *sync.Map,
	network string,
	loadCfg func(string) (*config.Config, error),
) (*grpc.ClientConn, error) {
	if v, ok := cache.Load(network); ok {
		return v.(*grpc.ClientConn), nil
	}

	cfg, err := loadCfg(network)
	if err != nil {
		return nil, err
	}

	cc, err := ClientConn(cfg)
	if err != nil {
		return nil, err
	}

	if actual, loaded := cache.LoadOrStore(network, cc); loaded {
		_ = cc.Close()
		return actual.(*grpc.ClientConn), nil
	}

	return cc, nil
}

// ClientConn creates a gRPC client connection from the given Config.
// It returns an error if the config does not contain exactly one endpoint.
func ClientConn(c *config.Config) (*grpc.ClientConn, error) {
	// no endpoints in config
	if len(c.Endpoints) != 1 {
		return nil, errors.New("we need a single endpoint")
	}

	// currently we only support connections to a single query service
	endpoint := c.Endpoints[0]

	// check endpoint address
	if len(endpoint.Address) == 0 {
		return nil, ErrInvalidAddress
	}

	// tls setup
	creds, err := TransportCredentials(endpoint.TLS)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract tls settings from config")
	}

	var opts []grpc.DialOption
	opts = append(opts, WithConnectionTime(endpoint.ConnectionTimeout))
	opts = append(opts, grpc.WithTransportCredentials(creds))

	return grpc.NewClient(endpoint.Address, opts...)
}

// TransportCredentials builds gRPC transport credentials from an endpoint's resolved TLS.
// Returns insecure credentials when TLS is disabled.
//
// It does not use SecureOptions.TLSConfig: this transport pins TLS 1.3, where the shared
// builder allows 1.2 for the surfaces that still need it. Only the material is shared.
//
// RootCAs is left nil when no anchors are configured, which makes crypto/tls fall back to
// the system root store rather than trusting nothing.
func TransportCredentials(opts grpc2.SecureOptions) (credentials.TransportCredentials, error) {
	if !opts.UseTLS {
		return insecure.NewCredentials(), nil
	}

	t := &tls.Config{
		MinVersion: tls.VersionTLS13,
		ServerName: opts.ServerNameOverride,
	}

	if len(opts.ServerRootCAs) > 0 {
		t.RootCAs = x509.NewCertPool()
		for _, rootCert := range opts.ServerRootCAs {
			if !t.RootCAs.AppendCertsFromPEM(rootCert) {
				return nil, errors.New("failed to parse a root certificate: not a valid PEM block")
			}
		}
	}

	// mTLS: both halves of the keypair must be present, or mutual TLS is skipped.
	if len(opts.Certificate) == 0 || len(opts.Key) == 0 {
		return credentials.NewTLS(t), nil
	}

	cert, err := tls.X509KeyPair(opts.Certificate, opts.Key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load client key pair")
	}
	t.Certificates = append(t.Certificates, cert)

	return credentials.NewTLS(t), nil
}

// WithConnectionTime returns a grpc.DialOption for setting the minimum connection timeout.
func WithConnectionTime(timeout time.Duration) grpc.DialOption {
	if timeout <= 0 {
		timeout = config.DefaultRequestTimeout
	}
	return grpc.WithConnectParams(grpc.ConnectParams{
		Backoff:           backoff.DefaultConfig,
		MinConnectTimeout: timeout,
	})
}
