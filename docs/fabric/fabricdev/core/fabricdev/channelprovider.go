/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricdev

import (
	"github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type provider struct {
	kvss                    *kvs.KVS
	publisher               events.Publisher
	hasher                  hash.Hasher
	newVault                generic.VaultConstructor
	tracerProvider          trace.TracerProvider
	drivers                 []driver2.NamedDriver
	channelConfigProvider   driver.ChannelConfigProvider
	listenerManagerProvider driver.ListenerManagerProvider
}

func NewChannelProvider(
	kvss *kvs.KVS,
	publisher events.Publisher,
	hasher hash.Hasher,
	tracerProvider trace.TracerProvider,
	drivers []driver2.NamedDriver,
	newVault generic.VaultConstructor,
	channelConfigProvider driver.ChannelConfigProvider,
	listenerManagerProvider driver.ListenerManagerProvider,
) *provider {
	return &provider{
		kvss:                    kvss,
		publisher:               publisher,
		hasher:                  hasher,
		newVault:                newVault,
		tracerProvider:          tracerProvider,
		drivers:                 drivers,
		channelConfigProvider:   channelConfigProvider,
		listenerManagerProvider: listenerManagerProvider,
	}
}

func (p *provider) NewChannel(nw driver.FabricNetworkService, channelName string, quiet bool) (driver.Channel, error) {
	networkOrderingService, ok := nw.(committer.OrderingService)
	if !ok {
		return nil, errors.Errorf("fabric network service does not implement committer.OrderingService")
	}

	// Channel configuration
	channelConfig, err := p.channelConfigProvider.GetChannelConfig(nw.Name(), channelName)
	if err != nil {
		return nil, err
	}

	// Vault
	vault, txIDStore, err := p.newVault(nw.ConfigService(), channelName, p.drivers)
	if err != nil {
		return nil, err
	}

	envelopeService := transaction.NewEnvelopeService(p.kvss, nw.Name(), channelName)
	transactionService := transaction.NewEndorseTransactionService(p.kvss, nw.Name(), channelName)
	metadataService := transaction.NewMetadataService(p.kvss, nw.Name(), channelName)
	peerService := peer.NewService(nw.ConfigService(), nw.LocalMembership().DefaultSigningIdentity())

	// Fabric finality
	fabricFinality, err := finality.NewFabricFinality(
		channelName,
		nw.ConfigService(),
		peerService,
		nw.LocalMembership().DefaultSigningIdentity(),
		p.hasher,
		channelConfig.FinalityWaitTimeout(),
	)
	if err != nil {
		return nil, err
	}

	channelMembershipService := membership.NewService()

	// Committers
	rwSetLoaderService := rwset.NewLoader(nw.Name(), channelName, envelopeService, transactionService, nw.TransactionManager(), vault)

	chaincodeManagerService := chaincode.NewManager(
		nw.Name(),
		channelName,
		nw.ConfigService(),
		channelConfig,
		channelConfig.GetNumRetries(),
		channelConfig.GetRetrySleep(),
		nw.LocalMembership(),
		peerService,
		nw.SignerService(),
		nw.OrderingService(),
		nil,
		channelMembershipService,
	)

	ledgerService := ledger.New()

	committerService := committer.New(
		nw.ConfigService(),
		channelConfig,
		vault,
		envelopeService,
		ledgerService,
		rwSetLoaderService,
		nw.ProcessorManager(),
		p.publisher,
		channelMembershipService,
		networkOrderingService,
		fabricFinality,
		channelConfig.CommitterWaitForEventTimeout(),
		nw.TransactionManager(),
		quiet,
		p.listenerManagerProvider.NewManager(),
		p.tracerProvider,
	)
	if err != nil {
		return nil, err
	}
	// Finality
	finalityService := committerService
	chaincodeManagerService.Finality = finalityService

	// Delivery
	deliveryService, err := delivery.NewService(
		channelName,
		channelConfig,
		p.hasher,
		nw.Name(),
		nw.LocalMembership(),
		nw.ConfigService(),
		peerService,
		ledgerService,
		channelConfig.CommitterWaitForEventTimeout(),
		txIDStore,
		nw.TransactionManager(),
		func(block *common.Block) (bool, error) {
			// commit the block, if an error occurs then retry
			return false, committerService.Commit(block)
		},
	)
	if err != nil {
		return nil, err
	}

	c := &generic.Channel{
		ChannelConfig:            channelConfig,
		ConfigService:            nw.ConfigService(),
		ChannelName:              channelName,
		FinalityService:          finalityService,
		VaultService:             vault,
		TXIDStoreService:         txIDStore,
		ES:                       envelopeService,
		TS:                       transactionService,
		MS:                       metadataService,
		DeliveryService:          deliveryService,
		RWSetLoaderService:       rwSetLoaderService,
		LedgerService:            ledgerService,
		ChannelMembershipService: channelMembershipService,
		ChaincodeManagerService:  chaincodeManagerService,
		CommitterService:         committerService,
		PeerManager:              peerService,
	}
	if err := c.Init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing Channel [%s]", channelName)
	}
	return c, nil
}
