/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"
	"reflect"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/deferred"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

//go:generate counterfeiter -o mock/grpc_client_provider.go --fake-name GRPCClientProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger.GRPCClientProvider
//go:generate counterfeiter -o mock/service_provider.go --fake-name ServicesProvider github.com/hyperledger-labs/fabric-smart-client/platform/view/services.Provider
//go:generate counterfeiter -o mock/block_query_client.go --fake-name BlockQueryServiceClient github.com/hyperledger/fabric-x-common/api/committerpb.BlockQueryServiceClient
//go:generate counterfeiter -o mock/query_client.go --fake-name QueryServiceClient github.com/hyperledger/fabric-x-common/api/committerpb.QueryServiceClient
//go:generate counterfeiter -o mock/config_provider.go --fake-name ConfigProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config.Provider
//go:generate counterfeiter -o mock/config_service.go --fake-name ConfigService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config.ConfigService
//go:generate counterfeiter -o mock/query_service.go --fake-name QueryService github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice.QueryService
//go:generate counterfeiter -o mock/query_service_provider.go --fake-name QueryServiceProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice.Provider

// GRPCClientProvider provides gRPC client connections for a given network.
//
//go:generate counterfeiter -o mock/grpc_client_provider.go --fake-name GRPCClientProvider . GRPCClientProvider
type GRPCClientProvider interface {
	// NotificationServiceClient returns a gRPC client connection for the specified network.
	NotificationServiceClient(network string) (*grpc.ClientConn, error)
}

// Provider provides ledger implementations to access transactions and blocks on the ledger.
//
// The base context the ledgers make their RPC calls on is not available when the
// Provider is built; the SDK supplies it during startup, via Initialize. Until
// then NewLedger and Context report driver.ErrNotInitialized.
type Provider struct {
	queryServiceProvider queryservice.Provider
	grpcClientProvider   GRPCClientProvider
	ledgers              lazy.Provider[string, driver.Ledger]

	// baseCtx holds the context installed by Initialize. A deferred.Holder
	// rather than a plain field guarded by sync.Once: Once orders its write
	// only against goroutines that call Do, and the readers here never do, so
	// it left them racing the write.
	baseCtx *deferred.Holder[context.Context]
}

// NewProvider creates a new Provider instance with the given gRPC client provider.
// The provider must be initialized with Initialize before use.
func NewProvider(grpcClientProvider GRPCClientProvider, queryServiceProvider queryservice.Provider) *Provider {
	p := &Provider{
		grpcClientProvider:   grpcClientProvider,
		queryServiceProvider: queryServiceProvider,
		baseCtx:              deferred.NewHolder[context.Context]("ledger provider base context"),
	}
	p.ledgers = lazy.NewProvider[string, driver.Ledger](func(s string) (driver.Ledger, error) {
		return p.newLedger(s)
	})
	return p
}

// Initialize sets the base context for the provider. This method must be called
// before NewLedger. It is safe to call multiple times; only the first call has effect.
//
// A nil context is ignored: installing one would leave the provider looking
// initialized while every ledger made its RPC calls on nil, which surfaces as a
// panic inside gRPC with nothing pointing back here. The provider stays
// uninitialized instead, so NewLedger and Context keep reporting it.
func (p *Provider) Initialize(ctx context.Context) {
	if ctx == nil {
		logger.Error("Ledger Provider.Initialize called with a nil context, ignoring it")
		return
	}

	// The error is discarded because the update function cannot fail; keeping
	// the first context is expressed by returning it unchanged.
	_ = p.baseCtx.Update(func(current context.Context, loaded bool) (context.Context, error) {
		if loaded {
			return current, nil
		}
		logger.Debug("Ledger Provider initialized with base context")
		return ctx, nil
	})
}

// NewLedger returns a ledger instance for the specified network.
// The channel parameter must be empty as FabricX does not support channels.
// It returns an error wrapping driver.ErrNotInitialized if Initialize has not
// run yet; newLedger reports that, before it opens any connection, on the first
// call for a network. Later calls are served from the cache, which cannot hold a
// ledger unless one was built — so unless Initialize had run.
func (p *Provider) NewLedger(network, channel string) (driver.Ledger, error) {
	return p.ledgers.Get(network)
}

// newLedger creates a new ledger instance for the specified network.
// It establishes a gRPC connection and creates the necessary client stubs.
func (p *Provider) newLedger(network string) (driver.Ledger, error) {
	baseCtx, err := p.baseCtx.Get()
	if err != nil {
		return nil, err
	}

	cc, err := p.grpcClientProvider.NotificationServiceClient(network)
	if err != nil {
		return nil, err
	}
	// Create the gRPC client stubs
	client := committerpb.NewBlockQueryServiceClient(cc)

	// get the query service
	qs, err := p.queryServiceProvider.Get(network, "")
	if err != nil {
		return nil, err
	}

	return New(client, qs, baseCtx), nil
}

// Context returns the base context used by the provider for RPC calls. It
// returns an error wrapping driver.ErrNotInitialized if Initialize has not run
// yet.
func (p *Provider) Context() (context.Context, error) {
	return p.baseCtx.Get()
}

// GetLedgerProvider fetches the Provider for the specified network and channel
func GetLedgerProvider(sp services.Provider) (*Provider, error) {
	lp, err := sp.GetService(reflect.TypeFor[*Provider]())
	if err != nil {
		return nil, errors.Wrapf(err, "could not find ledger provider")
	}
	return lp.(*Provider), nil
}
