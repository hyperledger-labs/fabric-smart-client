/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type VaultConstructor = func(
	channelName string,
	configService driver.ConfigService,
	vaultStore driver3.VaultStore,
	metricsProvider metrics.Provider,
	tracerProvider trace.TracerProvider,
) (*vault.Vault, error)
type LedgerConstructor func(
	channelName string,
	nw driver.FabricNetworkService,
	chaincodeManager driver.ChaincodeManager,
) (driver.Ledger, error)
type RWSetLoaderConstructor func(
	channel string,
	nw driver.FabricNetworkService,
	envelopeService driver.EnvelopeService,
	transactionService driver.EndorserTransactionService,
	vault driver.RWSetInspector,
) (driver.RWSetLoader, error)
type CommitterConstructor func(
	nw driver.FabricNetworkService,
	channelConfig driver.ChannelConfig,
	vault driver.Vault,
	envelopeService driver.EnvelopeService,
	ledger driver.Ledger,
	rwsetLoaderService driver.RWSetLoader,
	eventsPublisher events.Publisher,
	channelMembershipService *membership.Service,
	fabricFinality committer.FabricFinality,
	dependencyResolver committer.DependencyResolver,
	quiet bool,
	listenerManager driver.ListenerManager,
	tracerProvider trace.TracerProvider,
	metricsProvider metrics.Provider,
) (CommitterService, error)

type ChannelProvider interface {
	NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error)
}

type provider struct {
	envelopeKVS             driver.EnvelopeStore
	metadataKVS             driver.MetadataStore
	endorserTxKVS           driver.EndorseTxStore
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
	acceptedHeaderTypes     []common.HeaderType
}

func NewChannelProvider(
	envelopeKVS driver.EnvelopeStore,
	metadataKVS driver.MetadataStore,
	endorserTxKVS driver.EndorseTxStore,
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
	acceptedHeaderTypes []common.HeaderType,
) *provider {
	return &provider{
		envelopeKVS:             envelopeKVS,
		metadataKVS:             metadataKVS,
		endorserTxKVS:           endorserTxKVS,
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
		acceptedHeaderTypes:     acceptedHeaderTypes,
	}
}

func (p *provider) NewChannel(nw driver.FabricNetworkService, channelName string, quiet bool) (driver.Channel, error) {
	// Channel configuration
	channelConfig, err := p.channelConfigProvider.GetChannelConfig(nw.Name(), channelName)
	if err != nil {
		return nil, err
	}

	vaultStore, err := vault2.NewWithConfig(p.drivers, nw.ConfigService(), nw.Name(), channelName)
	if err != nil {
		return nil, err
	}

	vault, err := p.newVault(channelName, nw.ConfigService(), vaultStore, p.metricsProvider, p.tracerProvider)
	if err != nil {
		return nil, err
	}
	envelopeService := transaction.NewEnvelopeService(p.envelopeKVS, nw.Name(), channelName)
	transactionService := transaction.NewEndorseTransactionService(p.endorserTxKVS, nw.Name(), channelName)
	metadataService := transaction.NewMetadataService(p.metadataKVS, nw.Name(), channelName)
	peerService := services.NewClientFactory(nw.ConfigService(), nw.LocalMembership().DefaultSigningIdentity())

	// Fabric finality
	fabricFinality, err := finality.NewFabricFinality(
		logger,
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
	rwSetLoaderService, err := p.newRWSetLoader(
		channelName,
		nw,
		envelopeService,
		transactionService,
		vault,
	)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed creating RWSetLoader for channel [%s]", channelName)
	}

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

	ledgerService, err := p.newLedger(
		channelName,
		nw,
		chaincodeManagerService,
	)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed creating ledger for channel [%s]", channelName)
	}

	committerService, err := p.newCommitter(
		nw,
		channelConfig,
		vault,
		envelopeService,
		ledgerService,
		rwSetLoaderService,
		p.publisher,
		channelMembershipService,
		fabricFinality,
		p.dependencyResolver,
		quiet,
		p.listenerManagerProvider.NewManager(),
		p.tracerProvider,
		p.metricsProvider,
	)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed creating committer for channel [%s]", channelName)
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
		&fakeVault{vaultStore: vaultStore},
		nw.TransactionManager(),
		func(ctx context.Context, block *common.Block) (bool, error) {
			// commit the block, if an error occurs then retry
			return false, committerService.Commit(ctx, block)
		},
		p.tracerProvider,
		p.metricsProvider,
		p.acceptedHeaderTypes,
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
		VaultStoreService:        vaultStore,
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

type fakeVault struct {
	vaultStore driver3.VaultStore
}

func (f *fakeVault) GetLast(ctx context.Context) (*driver3.TxStatus, error) {
	return f.vaultStore.GetLast(ctx)
}

func (f *fakeVault) GetLastBlock(context.Context) (uint64, error) {
	return 0, errors.New("not implemented")
}
