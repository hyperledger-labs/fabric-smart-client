/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricdev

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/docs/platform/fabric/fabricdev/core/fabricdev/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
)

var logger = logging.MustGetLogger()

type provider struct {
	envelopeKVS             driver.EnvelopeStore
	metadataKVS             driver.MetadataStore
	endorseTxKVS            driver.EndorseTxStore
	publisher               events.Publisher
	hasher                  hash.Hasher
	newVault                generic.VaultConstructor
	tracerProvider          tracing.Provider
	metricsProvider         metrics.Provider
	dependencyResolver      committer.DependencyResolver
	drivers                 multiplexed.Driver
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
	tracerProvider tracing.Provider,
	metricsProvider metrics.Provider,
	drivers multiplexed.Driver,
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
	vaultStore, err := vault.NewStore(nw.ConfigService().VaultPersistenceName(), p.drivers, nw.Name(), channelName)
	if err != nil {
		return nil, err
	}

	vault, err := p.newVault(channelName, nw.ConfigService(), vaultStore)
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

	channelMembershipService := membership.NewService(channelName)

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
		&vaultDeliveryWrapper{vaultStore: vaultStore},
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
	}
	if err := c.Init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing Channel [%s]", channelName)
	}
	return c, nil
}

type vaultDeliveryWrapper struct {
	vaultStore driver3.VaultStore
}

func (f *vaultDeliveryWrapper) GetLastTxID(ctx context.Context) (string, error) {
	tx, err := f.vaultStore.GetLast(ctx)
	if err != nil {
		return "", err
	}

	if tx == nil {
		return "", nil
	}

	return tx.TxID, nil
}

func (f *vaultDeliveryWrapper) GetLastBlock(context.Context) (uint64, error) {
	return 0, errors.New("not implemented")
}
