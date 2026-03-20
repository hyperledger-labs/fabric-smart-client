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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
)

// Provider provides ledger implementations to access transactions and blocks on the ledger.
type Provider struct {
	fnsp           *fabric.NetworkServiceProvider
	configProvider config.Provider
	ledgers        map[string]driver.Ledger
	ledgersMu      sync.Mutex
	baseCtx        context.Context
	initOnce       sync.Once

	newLedger func(network, channel string, p *Provider) (driver.Ledger, error)
}

func NewProvider(fnsp *fabric.NetworkServiceProvider, configProvider config.Provider) *Provider {
	return &Provider{
		fnsp:           fnsp,
		configProvider: configProvider,
		ledgers:        make(map[string]driver.Ledger),
		newLedger:      newLedger,
	}
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

	key := network + ":" + channel
	p.ledgersMu.Lock()
	defer p.ledgersMu.Unlock()

	if l, ok := p.ledgers[key]; ok {
		return l, nil
	}

	l, err := p.newLedger(network, channel, p)
	if err != nil {
		return nil, err
	}
	p.ledgers[key] = l

	return l, nil
}

func newLedger(network, channel string, p *Provider) (driver.Ledger, error) {
	nw, err := p.fnsp.FabricNetworkService(network)
	if err != nil {
		return nil, err
	}

	// Load the specific configuration for this network
	cfg, err := p.configProvider.GetConfig(nw.Name())
	if err != nil {
		return nil, err
	}

	c, err := finality.NewConfig(cfg)
	if err != nil {
		return nil, err
	}

	cc, err := finality.GrpcClient(c)
	if err != nil {
		return nil, err
	}

	// Create the gRPC client stubs
	client := committerpb.NewBlockQueryServiceClient(cc)
	queryClient := committerpb.NewQueryServiceClient(cc)

	return New(client, queryClient, p.baseCtx), nil
}

// GetLedgerProvider fetches the Provider for the specified network and channel
func GetLedgerProvider(sp services.Provider) (*Provider, error) {
	lp, err := sp.GetService(reflect.TypeOf((*Provider)(nil)))
	if err != nil {
		return nil, errors.Wrapf(err, "could not find ledger provider")
	}
	return lp.(*Provider), nil
}
