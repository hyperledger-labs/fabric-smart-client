/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/lazy"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// Provider provides ledger implementations to access transactions and blocks on the ledger.
// The ledger can be accessed in various ways, e.g. using the chaincode or listening on the delivery endpoint
// However, FabricX does not invoke the chaincode to access blocks and transactions.
type Provider interface {
	NewLedger(network, channel string) (driver.Ledger, error)
}

type eventBasedProvider struct {
	bdp *BlockDispatcherProvider
}

func NewEventBasedProvider(
	bdp *BlockDispatcherProvider,
) *eventBasedProvider {
	return &eventBasedProvider{
		bdp: bdp,
	}
}

func (p *eventBasedProvider) NewLedger(network, channel string) (driver.Ledger, error) {
	dispatcher, err := p.bdp.GetBlockDispatcher(network, channel)
	if err != nil {
		return nil, err
	}

	// this ledger attaches to the delivery service via the block dispatcher
	l := New()
	dispatcher.AddCallback(l.OnBlock)

	return l, nil
}

func key(k netCh) string { return k.network + "," + k.channel }

func NewBlockDispatcherProvider() *BlockDispatcherProvider {
	return &BlockDispatcherProvider{Provider: lazy.NewProviderWithKeyMapper(key, func(k netCh) (*BlockDispatcher, error) {
		return &BlockDispatcher{}, nil
	})}
}

type netCh struct{ network, channel string }

type BlockDispatcherProvider struct {
	lazy.Provider[netCh, *BlockDispatcher]
}

func (p *BlockDispatcherProvider) GetBlockDispatcher(network, channel string) (*BlockDispatcher, error) {
	return p.Get(netCh{network, channel})
}
