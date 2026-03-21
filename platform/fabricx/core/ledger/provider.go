/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"
	"reflect"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/grpc"
)

//go:generate counterfeiter -o mock/grpc_client_provider.go --fake-name GRPCClientProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger.GRPCClientProvider
//go:generate counterfeiter -o mock/service_provider.go --fake-name ServicesProvider github.com/hyperledger-labs/fabric-smart-client/platform/view/services.Provider
//go:generate counterfeiter -o mock/block_query_client.go --fake-name BlockQueryServiceClient github.com/hyperledger/fabric-x-common/api/committerpb.BlockQueryServiceClient
//go:generate counterfeiter -o mock/query_client.go --fake-name QueryServiceClient github.com/hyperledger/fabric-x-common/api/committerpb.QueryServiceClient
//go:generate counterfeiter -o mock/config_provider.go --fake-name ConfigProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config.Provider
//go:generate counterfeiter -o mock/config_service.go --fake-name ConfigService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config.ConfigService

//go:generate counterfeiter -o mock/grpc_client_provider.go --fake-name GRPCClientProvider . GRPCClientProvider
type GRPCClientProvider interface {
	Client(network string) (*grpc.ClientConn, error)
}

// Provider provides ledger implementations to access transactions and blocks on the ledger.
type Provider struct {
	grpcClientProvider GRPCClientProvider
	ledgers            lazy.Provider[string, driver.Ledger]
	baseCtx            context.Context
	initOnce           sync.Once
}

func NewProvider(grpcClientProvider GRPCClientProvider) *Provider {
	p := &Provider{
		grpcClientProvider: grpcClientProvider,
	}
	p.ledgers = lazy.NewProvider[string, driver.Ledger](func(s string) (driver.Ledger, error) {
		return p.newLedger(s)
	})
	return p
}

func (p *Provider) Initialize(ctx context.Context) {
	p.initOnce.Do(func() {
		p.baseCtx = ctx
		logger.Debug("Ledger Provider initialized with base context")
	})
}

func (p *Provider) NewLedger(network, channel string) (driver.Ledger, error) {
	if p.baseCtx == nil {
		panic("programming error: Provider is not initialized. The Initialize() method must be called before NewLedger.")
	}
	if len(channel) != 0 {
		return nil, errors.Errorf("non-empty channel not supported")
	}

	return p.ledgers.Get(network)
}

func (p *Provider) newLedger(network string) (driver.Ledger, error) {
	cc, err := p.grpcClientProvider.Client(network)
	if err != nil {
		return nil, err
	}
	// Create the gRPC client stubs
	client := committerpb.NewBlockQueryServiceClient(cc)
	queryClient := committerpb.NewQueryServiceClient(cc)

	return New(client, queryClient, p.baseCtx), nil
}

func (p *Provider) Context() context.Context {
	return p.baseCtx
}

// GetLedgerProvider fetches the Provider for the specified network and channel
func GetLedgerProvider(sp services.Provider) (*Provider, error) {
	lp, err := sp.GetService(reflect.TypeOf((*Provider)(nil)))
	if err != nil {
		return nil, errors.Wrapf(err, "could not find ledger provider")
	}
	return lp.(*Provider), nil
}
