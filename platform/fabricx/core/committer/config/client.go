/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commongrpc "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
)

// ServiceConfigProvider provides gRPC configuration for a given network. Satisfied by
// *Provider.
//
//go:generate counterfeiter -o mock/service_config_provider.go --fake-name ServiceConfigProvider . ServiceConfigProvider
type ServiceConfigProvider interface {
	// NotificationServiceConfig returns the configuration for the notification service for the specified network.
	NotificationServiceConfig(network string) (*Config, error)
	// QueryServiceConfig returns the configuration for the query service for the specified network.
	QueryServiceConfig(network string) (*Config, error)
}

// ClientProvider hands out gRPC connections to the committer's services, one per
// (service, network).
//
// The connection is cached because a fresh one per call leaks a connection's worth of
// goroutines every time: nothing closes what a Get returns. lazy.Provider is the same
// caching the Fabric peer and orderer clients use (see
// platform/fabric/core/generic/services/conn.go).
type ClientProvider struct {
	notification lazy.Provider[string, *grpc.ClientConn]
	query        lazy.Provider[string, *grpc.ClientConn]
}

// NewClientProvider returns a new ClientProvider instance.
func NewClientProvider(configProvider ServiceConfigProvider) *ClientProvider {
	return &ClientProvider{
		notification: lazy.NewProvider(dial(configProvider.NotificationServiceConfig)),
		query:        lazy.NewProvider(dial(configProvider.QueryServiceConfig)),
	}
}

// NotificationServiceClient returns the gRPC connection to the notification service for the
// specified network, dialling it on first use.
func (c *ClientProvider) NotificationServiceClient(network string) (*grpc.ClientConn, error) {
	return c.notification.Get(network)
}

// QueryServiceClient returns the gRPC connection to the query service for the specified
// network, dialling it on first use.
func (c *ClientProvider) QueryServiceClient(network string) (*grpc.ClientConn, error) {
	return c.query.Get(network)
}

// dial turns a per-network config lookup into the connection factory lazy.Provider caches.
// CreateGRPCClient then NewConnection gives the committer's services TLS version selection
// and message-size configuration through the shared client. Unlike the old dialer, this
// connects eagerly: the first call per network blocks for up to connectionTimeout (default
// 5s) if the sidecar isn't reachable, instead of dialing lazily and failing on the first RPC.
func dial(loadCfg func(string) (*Config, error)) func(string) (*grpc.ClientConn, error) {
	return func(network string) (*grpc.ClientConn, error) {
		cfg, err := loadCfg(network)
		if err != nil {
			return nil, err
		}
		// Only a single endpoint per service is supported.
		if len(cfg.Endpoints) != 1 {
			return nil, errors.Errorf("expected exactly one endpoint for [%s], got %d",
				network, len(cfg.Endpoints))
		}
		endpoint := cfg.Endpoints[0]
		client, err := commongrpc.CreateGRPCClient(&endpoint)
		if err != nil {
			return nil, errors.Wrapf(err, "failed creating grpc client for [%s]", endpoint.Address)
		}
		return client.NewConnection(endpoint.Address)
	}
}
