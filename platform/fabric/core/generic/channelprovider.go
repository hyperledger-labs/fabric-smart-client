/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type VaultConstructor = func(configService driver.ConfigService, channel string, drivers []driver2.NamedDriver, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) (*vault.Vault, driver.TXIDStore, error)
type LedgerConstructor func(
	channelName string,
	chaincodeManager driver.ChaincodeManager,
	localMembership driver.LocalMembership,
	configService driver.ConfigService,
	transactionManager driver.TransactionManager,
) driver.Ledger
type RWSetLoaderConstructor func(
	network string,
	channel string,
	envelopeService driver.EnvelopeService,
	transactionService driver.EndorserTransactionService,
	transactionManager driver.TransactionManager,
	vault driver.RWSetInspector,
) driver.RWSetLoader
type CommitterConstructor func(
	configService driver.ConfigService,
	channelConfig driver.ChannelConfig,
	vault driver.Vault,
	envelopeService driver.EnvelopeService,
	ledger driver.Ledger,
	rwsetLoaderService driver.RWSetLoader,
	processorManager driver.ProcessorManager,
	eventsPublisher events.Publisher,
	channelMembershipService *membership.Service,
	orderingService committer.OrderingService,
	fabricFinality committer.FabricFinality,
	transactionManager driver.TransactionManager,
	dependencyResolver committer.DependencyResolver,
	quiet bool,
	listenerManager driver.ListenerManager,
	tracerProvider trace.TracerProvider,
	metricsProvider metrics.Provider,
) *committer.Committer

type ChannelProvider interface {
	NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error)
}

type provider struct {
	kvss                    *kvs.KVS
	publisher               events.Publisher
	hasher                  hash.Hasher
	newVault                VaultConstructor
	tracerProvider          trace.TracerProvider
	metricsProvider         metrics.Provider
	dependencyResolver      committer.DependencyResolver
	drivers                 []driver2.NamedDriver
	channelConfigProvider   driver.ChannelConfigProvider
	listenerManagerProvider driver.ListenerManagerProvider
	newLedger               LedgerConstructor
	newRWSetLoader          RWSetLoaderConstructor
	newCommitter            CommitterConstructor
	useFilteredDelivery     bool
}

func NewChannelProvider(
	kvss *kvs.KVS,
	publisher events.Publisher,
	hasher hash.Hasher,
	tracerProvider trace.TracerProvider,
	metricsProvider metrics.Provider,
	drivers []driver2.NamedDriver,
	newVault VaultConstructor,
	channelConfigProvider driver.ChannelConfigProvider,
	listenerManagerProvider driver.ListenerManagerProvider,
	dependencyResolver committer.DependencyResolver,
	newLedger LedgerConstructor,
	newRWSetLoader RWSetLoaderConstructor,
	newCommitter CommitterConstructor,
	useFilteredDelivery bool,
) *provider {
	return &provider{
		kvss:                    kvss,
		publisher:               publisher,
		hasher:                  hasher,
		newVault:                newVault,
		tracerProvider:          tracerProvider,
		metricsProvider:         metricsProvider,
		drivers:                 drivers,
		channelConfigProvider:   channelConfigProvider,
		listenerManagerProvider: listenerManagerProvider,
		dependencyResolver:      dependencyResolver,
		newLedger:               newLedger,
		newRWSetLoader:          newRWSetLoader,
		newCommitter:            newCommitter,
		useFilteredDelivery:     useFilteredDelivery,
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
	vault, txIDStore, err := p.newVault(nw.ConfigService(), channelName, p.drivers, p.metricsProvider, p.tracerProvider)
	if err != nil {
		return nil, err
	}

	envelopeService := transaction.NewEnvelopeService(p.kvss, nw.Name(), channelName)
	transactionService := transaction.NewEndorseTransactionService(p.kvss, nw.Name(), channelName)
	metadataService := transaction.NewMetadataService(p.kvss, nw.Name(), channelName)
	peerService := services.NewClientFactory(nw.ConfigService(), nw.LocalMembership().DefaultSigningIdentity())

	// Fabric finality
	fabricFinality, err := finality.NewFabricFinality(
		channelName,
		nw.ConfigService(),
		peerService,
		nw.LocalMembership().DefaultSigningIdentity(),
		p.hasher,
		channelConfig.FinalityWaitTimeout(),
		p.useFilteredDelivery,
	)
	if err != nil {
		return nil, err
	}

	channelMembershipService := membership.NewService()

	// Committers
	rwSetLoaderService := p.newRWSetLoader(nw.Name(), channelName, envelopeService, transactionService, nw.TransactionManager(), vault)

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

	ledgerService := p.newLedger(
		channelName,
		chaincodeManagerService,
		nw.LocalMembership(),
		nw.ConfigService(),
		nw.TransactionManager(),
	)

	committerService := p.newCommitter(
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
		nw.TransactionManager(),
		p.dependencyResolver,
		quiet,
		p.listenerManagerProvider.NewManager(),
		p.tracerProvider,
		p.metricsProvider,
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
		func(ctx context.Context, block *common.Block) (bool, error) {
			// commit the block, if an error occurs then retry
			return false, committerService.Commit(ctx, block)
		},
		p.tracerProvider,
		p.metricsProvider,
	)
	if err != nil {
		return nil, err
	}

	c := &Channel{
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
		PeerService:              peerService,
	}
	if err := c.Init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing Channel [%s]", channelName)
	}
	return c, nil
}
