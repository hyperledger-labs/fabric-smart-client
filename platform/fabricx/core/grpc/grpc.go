/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"os"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// ErrInvalidAddress is returned when an endpoint address is empty.
var ErrInvalidAddress = errors.New("empty address")

// ClientProvider provides gRPC client connections for a given network.
type ClientProvider struct {
	// configProvider is used to retrieve the configuration for a network.
	configProvider config.Provider
}

// NewClientProvider returns a new ClientProvider instance.
func NewClientProvider(configProvider config.Provider) *ClientProvider {
	return &ClientProvider{configProvider: configProvider}
}

// Client returns a gRPC client connection for the specified network.
// It loads the configuration for the network and creates a connection.
func (c *ClientProvider) Client(network string) (*grpc.ClientConn, error) {
	// Load the specific configuration for this network
	cfg, err := c.configProvider.GetConfig(network)
	if err != nil {
		return nil, err
	}

	config, err := NewConfig(cfg)
	if err != nil {
		return nil, err
	}

	cc, err := ClientConn(config)
	if err != nil {
		return nil, err
	}

	return cc, nil
}

// ClientConn creates a gRPC client connection from the given Config.
// It returns an error if the config does not contain exactly one endpoint.
func ClientConn(c *Config) (*grpc.ClientConn, error) {
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

	var opts []grpc.DialOption
	opts = append(opts, WithConnectionTime(endpoint.ConnectionTimeout))
	opts = append(opts, WithTLS(endpoint))

	return grpc.NewClient(endpoint.Address, opts...)
}

// WithTLS returns a grpc.DialOption for configuring TLS based on the given endpoint.
func WithTLS(endpoint Endpoint) grpc.DialOption {
	if !endpoint.TLSEnabled {
		return grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	if _, err := os.Stat(endpoint.TLSRootCertFile); errors.Is(err, os.ErrNotExist) {
		if err != nil {
			panic(err)
		}
	}

	creds, err := credentials.NewClientTLSFromFile(endpoint.TLSRootCertFile, endpoint.TLSServerNameOverride)
	if err != nil {
		panic(err)
	}

	return grpc.WithTransportCredentials(creds)
}

// WithConnectionTime returns a grpc.DialOption for setting the minimum connection timeout.
func WithConnectionTime(timeout time.Duration) grpc.DialOption {
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	return grpc.WithConnectParams(grpc.ConnectParams{
		Backoff:           backoff.DefaultConfig,
		MinConnectTimeout: timeout,
	})
}
