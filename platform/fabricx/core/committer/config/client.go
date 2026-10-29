/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"cmp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

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

// ClientProvider hands out gRPC connections to the committer's services. For each
// (service, network) it holds one connection per configured endpoint and, on every call,
// hands out the first one that is not failing (see pick). Callers that ask again after a
// failure therefore move to another endpoint, and back once gRPC has reconnected the
// earlier one.
//
// The connections are cached for the life of the node because a fresh one per call leaks a
// connection's worth of goroutines every time: nothing closes what a Get returns.
// lazy.Provider is the same caching the Fabric peer and orderer clients use (see
// platform/fabric/core/generic/services/conn.go).
type ClientProvider struct {
	notification lazy.Provider[string, []*grpc.ClientConn]
	query        lazy.Provider[string, []*grpc.ClientConn]
}

// NewClientProvider returns a new ClientProvider instance.
func NewClientProvider(configProvider ServiceConfigProvider) *ClientProvider {
	return &ClientProvider{
		notification: lazy.NewProvider(dial(configProvider.NotificationServiceConfig)),
		query:        lazy.NewProvider(dial(configProvider.QueryServiceConfig)),
	}
}

// NotificationServiceClient returns a gRPC connection to the notification service for the
// specified network, dialling every endpoint on first use. The connection is chosen per call,
// so a caller should ask again after a failure instead of keeping the one it got.
func (c *ClientProvider) NotificationServiceClient(network string) (*grpc.ClientConn, error) {
	conns, err := c.notification.Get(network)
	if err != nil {
		return nil, err
	}
	return pick(conns), nil
}

// QueryServiceClient returns a gRPC connection to the query service for the specified
// network, dialling every endpoint on first use. The connection is chosen per call, so a
// caller should ask again after a failure instead of keeping the one it got.
func (c *ClientProvider) QueryServiceClient(network string) (*grpc.ClientConn, error) {
	conns, err := c.query.Get(network)
	if err != nil {
		return nil, err
	}
	return pick(conns), nil
}

// dial turns a per-network config lookup into the connection factory lazy.Provider caches.
// NewGRPCClient then NewConnection gives the committer's services TLS version selection
// and message-size configuration through the shared client.
//
// Every endpoint gets its own connection, returned in configuration order. Connecting does
// not block, so an endpoint that is down at startup still gets a connection that gRPC keeps
// retrying in the background, and startup does not wait on unreachable endpoints. An error
// is returned only if no endpoint yields a connection, and it joins every endpoint's failure.
func dial(loadCfg func(string) (*Config, error)) func(string) ([]*grpc.ClientConn, error) {
	return func(network string) ([]*grpc.ClientConn, error) {
		cfg, err := loadCfg(network)
		if err != nil {
			return nil, err
		}
		if len(cfg.Endpoints) == 0 {
			return nil, errors.Errorf("expected at least one endpoint for [%s]", network)
		}

		var conns []*grpc.ClientConn
		var errs []error
		for _, endpoint := range cfg.Endpoints {
			// CreateGRPCClient has no way to set AsyncConnect, so build the ClientConfig here.
			client, err := commongrpc.NewGRPCClient(commongrpc.ClientConfig{
				Timeout:        cmp.Or(endpoint.ConnectionTimeout, commongrpc.DefaultConnectionTimeout),
				SecOpts:        endpoint.TLS,
				MaxRecvMsgSize: endpoint.MaxRecvMsgSize,
				MaxSendMsgSize: endpoint.MaxSendMsgSize,
				// Without blocking, an endpoint that is down at startup still gets a conn that gRPC keeps retrying.
				AsyncConnect: true,
			})
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed creating grpc client for [%s]", endpoint.Address))
				continue
			}

			conn, err := client.NewConnection(endpoint.Address)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			conns = append(conns, conn)
		}
		if len(conns) != 0 {
			return conns, nil
		}
		return nil, errors.Join(errs...)
	}
}

// pick returns the first conn, in configuration order, that is not in TRANSIENT_FAILURE,
// or the first conn if all are. A conn whose server has gone away reports IDLE until a
// call on it fails, so it can still be returned once; callers that need the next endpoint
// then, such as the query service, pick again after an Unavailable error.
func pick(connections []*grpc.ClientConn) *grpc.ClientConn {
	for _, conn := range connections {
		if conn.GetState() != connectivity.TransientFailure {
			return conn
		}
	}
	return connections[0]
}
