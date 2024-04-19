/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

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
	Network                  *Network
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
	CommitterService         *committer.Service
	PeerManager              *PeerManager
}

func NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error) {
	network := nw.(*Network)
	sp := network.SP

	// Channel configuration
	channelConfig := network.ConfigService().Channel(name)
	if channelConfig == nil {
		channelConfig = network.ConfigService().NewDefaultChannelConfig(name)
	}

	// Vault
	v, txIDStore, err := NewVault(sp, network.configService, name)
	if err != nil {
		return nil, err
	}

	// Events
	eventsPublisher, err := events.GetPublisher(sp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get event publisher")
	}

	kvsService := kvs.GetService(sp)

	ChannelName := name
	ConfigService := network.configService
	Network := network
	VaultService := NewVaultService(v)
	TXIDStoreService := txIDStore
	ES := transaction.NewEnvelopeService(kvsService, network.Name(), name)
	TS := transaction.NewEndorseTransactionService(kvsService, network.Name(), name)
	MS := transaction.NewMetadataService(kvsService, network.Name(), name)
	PeerManager := NewPeerManager(network.configService, network.LocalMembership().DefaultSigningIdentity())

	// Fabric finality
	fabricFinality, err := finality.NewFabricFinality(
		name,
		network.ConfigService(),
		PeerManager,
		network.LocalMembership().DefaultSigningIdentity(),
		hash.GetHasher(sp),
		channelConfig.FinalityWaitTimeout(),
	)
	if err != nil {
		return nil, err
	}

	ChannelMembershipService := membership.NewService()

	// Committers
	RWSetLoaderService := NewRWSetLoader(
		network.Name(), name,
		ES, TS, network.TransactionManager(),
		v,
	)

	ChaincodeManagerService := NewChaincodeManager(
		network.Name(),
		name,
		network.configService,
		channelConfig,
		channelConfig.GetNumRetries(),
		channelConfig.GetRetrySleep(),
		network.localMembership,
		PeerManager,
		network.sigService,
		network.Ordering,
		nil,
		ChannelMembershipService,
	)

	LedgerService := NewLedger(
		name,
		ChaincodeManagerService,
		network.localMembership,
		network.configService,
	)

	CommitterService := committer.NewService(
		network.configService,
		channelConfig,
		VaultService,
		ES,
		LedgerService,
		RWSetLoaderService,
		Network.processorManager,
		eventsPublisher,
		ChannelMembershipService,
		Network,
		fabricFinality,
		channelConfig.CommitterWaitForEventTimeout(),
		quiet,
		tracing.Get(sp).GetTracer(),
	)
	if err != nil {
		return nil, err
	}
	// Finality
	FinalityService := CommitterService
	ChaincodeManagerService.Finality = FinalityService

	// Delivery
	deliveryService, err := NewDeliveryService(
		name,
		channelConfig,
		hash.GetHasher(sp),
		network.Name(),
		network.LocalMembership(),
		network.ConfigService(),
		PeerManager,
		LedgerService,
		channelConfig.CommitterWaitForEventTimeout(),
		txIDStore,
		func(block *common.Block) (bool, error) {
			// commit the block, if an error occurs then retry
			err := CommitterService.Commit(block)
			return false, err
		},
	)
	if err != nil {
		return nil, err
	}

	c := &Channel{
		ChannelConfig:            channelConfig,
		ConfigService:            ConfigService,
		Network:                  network,
		ChannelName:              ChannelName,
		FinalityService:          FinalityService,
		VaultService:             VaultService,
		TXIDStoreService:         TXIDStoreService,
		ES:                       ES,
		TS:                       TS,
		MS:                       MS,
		DeliveryService:          deliveryService,
		RWSetLoaderService:       RWSetLoaderService,
		LedgerService:            LedgerService,
		ChannelMembershipService: ChannelMembershipService,
		ChaincodeManagerService:  ChaincodeManagerService,
		CommitterService:         CommitterService,
		PeerManager:              PeerManager,
	}
	if err := c.Init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing Channel [%s]", name)
	}
	return c, nil
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
