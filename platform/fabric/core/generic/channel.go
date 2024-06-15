/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
)

type committerService interface {
	ReloadConfigTransactions() error
	driver.Committer
}

// These are function names from Invoke first parameter
const (
	GetBlockByNumber   string = "GetBlockByNumber"
	GetTransactionByID string = "GetTransactionByID"
	GetBlockByTxID     string = "GetBlockByTxID"
)

type Delivery interface {
	Start(ctx context.Context)
	Stop()
}

type Channel struct {
	ChannelConfig            driver.ChannelConfig
	ConfigService            driver.ConfigService
	ChannelName              string
	FinalityService          driver.Finality
	VaultService             driver.Vault
	TXIDStoreService         driver.TXIDStore
	ES                       driver.EnvelopeService
	TS                       driver.EndorserTransactionService
	MS                       driver.MetadataService
	DeliveryService          *DeliveryService
	RWSetLoaderService       driver.RWSetLoader
	LedgerService            driver.Ledger
	ChannelMembershipService *membership.Service
	ChaincodeManagerService  driver.ChaincodeManager
	CommitterService         committerService
	PeerManager              *PeerManager
}

func NewChannel(sp view.ServiceProvider, nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error) {
	publisher, err := events.GetPublisher(sp)
	if err != nil {
		return nil, err
	}
	p := NewProvider(kvs.GetService(sp), publisher, hash.GetHasher(sp), tracing.Get(sp))
	return p.NewChannel(nw, name, quiet)
}

func (c *Channel) Name() string {
	return c.ChannelName
}

func (c *Channel) Close() error {
	c.DeliveryService.Stop()
	return c.Vault().Close()
}

func (c *Channel) Vault() driver.Vault {
	return c.VaultService
}

func (c *Channel) Finality() driver.Finality {
	return c.FinalityService
}

func (c *Channel) Ledger() driver.Ledger {
	return c.LedgerService
}

func (c *Channel) Delivery() driver.Delivery {
	return c.DeliveryService
}

func (c *Channel) ChaincodeManager() driver.ChaincodeManager {
	return c.ChaincodeManagerService
}

func (c *Channel) ChannelMembership() driver.ChannelMembership {
	return c.ChannelMembershipService
}

func (c *Channel) TXIDStore() driver.TXIDStore {
	return c.TXIDStoreService
}

func (c *Channel) RWSetLoader() driver.RWSetLoader {
	return c.RWSetLoaderService
}

func (c *Channel) Committer() driver.Committer {
	return c.CommitterService
}

func (c *Channel) Init() error {
	if err := c.CommitterService.ReloadConfigTransactions(); err != nil {
		return errors.WithMessagef(err, "failed reloading config transactions")
	}
	return nil
}
