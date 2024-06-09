/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type VaultConstructor = func(configService driver.ConfigService, channel string) (*vault.Vault, TXIDStore, error)

type Provider interface {
	NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error)
}

func NewProvider(kvss *kvs.KVS, publisher events.Publisher, hasher hash.Hasher, tracerProvider trace.TracerProvider) Provider {
	return NewProviderWithVault(kvss, publisher, hasher, tracerProvider, NewVault)
}

func NewProviderWithVault(kvss *kvs.KVS, publisher events.Publisher, hasher hash.Hasher, tracerProvider trace.TracerProvider, newVault VaultConstructor) *provider {
	return &provider{kvss: kvss, publisher: publisher, hasher: hasher, newVault: newVault, tracerProvider: tracerProvider}
}

type provider struct {
	kvss           *kvs.KVS
	publisher      events.Publisher
	hasher         hash.Hasher
	newVault       VaultConstructor
	tracerProvider trace.TracerProvider
}

func (p *provider) NewChannel(nw driver.FabricNetworkService, channelName string, quiet bool) (driver.Channel, error) {
	// Channel configuration
	channelConfig := nw.ConfigService().Channel(channelName)
	if channelConfig == nil {
		channelConfig = nw.ConfigService().NewDefaultChannelConfig(channelName)
	}

	// Vault
	vault, txIDStore, err := p.newVault(nw.ConfigService(), channelName)
	if err != nil {
		return nil, err
	}
	vaultService := NewVaultService(vault)
	envelopeService := transaction.NewEnvelopeService(p.kvss, nw.Name(), channelName)
	transactionService := transaction.NewEndorseTransactionService(p.kvss, nw.Name(), channelName)
	metadataService := transaction.NewMetadataService(p.kvss, nw.Name(), channelName)
	peerManager := NewPeerManager(nw.ConfigService(), nw.LocalMembership().DefaultSigningIdentity())

	// Fabric finality
	fabricFinality, err := finality.NewFabricFinality(
		channelName,
		nw.ConfigService(),
		peerManager,
		nw.LocalMembership().DefaultSigningIdentity(),
		p.hasher,
		channelConfig.FinalityWaitTimeout(),
	)
	if err != nil {
		return nil, err
	}

	channelMembershipService := membership.NewService()

	// Committers
	rwSetLoaderService := NewRWSetLoader(nw.Name(), channelName, envelopeService, transactionService, nw.TransactionManager(), vault)

	chaincodeManagerService := NewChaincodeManager(
		nw.Name(),
		channelName,
		nw.ConfigService(),
		channelConfig,
		channelConfig.GetNumRetries(),
		channelConfig.GetRetrySleep(),
		nw.LocalMembership(),
		peerManager,
		nw.SignerService(),
		nw.OrderingService(),
		nil,
		channelMembershipService,
	)

	ledgerService := NewLedger(
		channelName,
		chaincodeManagerService,
		nw.LocalMembership(),
		nw.ConfigService(),
	)

	committerService := committer.NewService(
		nw.ConfigService(),
		channelConfig,
		vaultService,
		envelopeService,
		ledgerService,
		rwSetLoaderService,
		nw.ProcessorManager(),
		p.publisher,
		channelMembershipService,
		nw.(*Network),
		fabricFinality,
		channelConfig.CommitterWaitForEventTimeout(),
		quiet,
		p.tracerProvider,
	)
	if err != nil {
		return nil, err
	}
	// Finality
	finalityService := committerService
	chaincodeManagerService.Finality = finalityService

	// Delivery
	deliveryService, err := NewDeliveryService(
		channelName,
		channelConfig,
		p.hasher,
		nw.Name(),
		nw.LocalMembership(),
		nw.ConfigService(),
		peerManager,
		ledgerService,
		channelConfig.CommitterWaitForEventTimeout(),
		txIDStore,
		func(block *common.Block) (bool, error) {
			// commit the block, if an error occurs then retry
			return false, committerService.Commit(block)
		},
	)
	if err != nil {
		return nil, err
	}

	c := &Channel{
		ChannelConfig:            channelConfig,
		ConfigService:            nw.ConfigService(),
		ChannelName:              channelName,
		FinalityService:          finalityService,
		VaultService:             vaultService,
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
		PeerManager:              peerManager,
	}
	if err := c.Init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing Channel [%s]", channelName)
	}
	return c, nil
}
