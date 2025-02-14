/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricdev

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/docs/fabric/fabricdev/core/fabricdev/ledger"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger("fabric-sdk.core")

type provider struct {
	envelopeKVS             driver.EnvelopeStore
	metadataKVS             driver.MetadataStore
	endorseTxKVS            driver.EndorseTxStore
	publisher               events.Publisher
	hasher                  hash.Hasher
	newVault                generic.VaultConstructor
	tracerProvider          trace.TracerProvider
	metricsProvider         metrics.Provider
	dependencyResolver      committer.DependencyResolver
	drivers                 []driver2.NamedDriver
	channelConfigProvider   driver.ChannelConfigProvider
	listenerManagerProvider driver.ListenerManagerProvider
	acceptedHeaderTypes     []common.HeaderType
}

func NewChannelProvider(
	envelopeKVS driver.EnvelopeStore,
	metadataKVS driver.MetadataStore,
	endorseTxKVS driver.EndorseTxStore,
	publisher events.Publisher,
	hasher hash.Hasher,
	tracerProvider trace.TracerProvider,
	metricsProvider metrics.Provider,
	drivers []driver2.NamedDriver,
	newVault generic.VaultConstructor,
	channelConfigProvider driver.ChannelConfigProvider,
	listenerManagerProvider driver.ListenerManagerProvider,
	dependencyResolver committer.DependencyResolver,
	acceptedHeaderTypes []common.HeaderType,
) *provider {
	return &provider{
		envelopeKVS:             envelopeKVS,
		metadataKVS:             metadataKVS,
		endorseTxKVS:            endorseTxKVS,
		publisher:               publisher,
		hasher:                  hasher,
		newVault:                newVault,
		tracerProvider:          tracerProvider,
		metricsProvider:         metricsProvider,
		drivers:                 drivers,
		channelConfigProvider:   channelConfigProvider,
		listenerManagerProvider: listenerManagerProvider,
		dependencyResolver:      dependencyResolver,
		acceptedHeaderTypes:     acceptedHeaderTypes,
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
	vaultStore, err := vault.NewWithConfig(p.drivers, nw.ConfigService(), nw.Name(), channelName)
	if err != nil {
		return nil, err
	}

	vault, err := p.newVault(channelName, nw.ConfigService(), vaultStore, p.metricsProvider, p.tracerProvider)
	if err != nil {
		return nil, err
	}
	envelopeService := transaction.NewEnvelopeService(p.envelopeKVS, nw.Name(), channelName)
	transactionService := transaction.NewEndorseTransactionService(p.endorseTxKVS, nw.Name(), channelName)
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
		true,
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

	c := &generic.Channel{
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
