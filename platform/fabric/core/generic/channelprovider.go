/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
)

type VaultConstructor = func(
	channelName string,
	configService driver.ConfigService,
	vaultStore driver3.VaultStore,
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
	channel string,
	vault driver.Vault,
	envelopeService driver.EnvelopeService,
	ledger driver.Ledger,
	rwsetLoaderService driver.RWSetLoader,
	channelMembershipService driver.MembershipService,
	fabricFinality committer.FabricFinality,
	quiet bool,
) (CommitterService, error)

type DeliveryConstructor func(
	nw driver.FabricNetworkService,
	channel string,
	peerManager delivery.Services,
	ledger driver.Ledger,
	vault delivery.Vault,
	callback driver.BlockCallback,
) (DeliveryService, error)

type MembershipConstructor func(channelName string) driver.MembershipService

type ChannelProvider interface {
	NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error)
}

type provider struct {
	envelopeKVS           driver.EnvelopeStore
	metadataKVS           driver.MetadataStore
	endorserTxKVS         driver.EndorseTxStore
	hasher                hash.Hasher
	newVault              VaultConstructor
	drivers               multiplexed.Driver
	channelConfigProvider driver.ChannelConfigProvider
	newLedger             LedgerConstructor
	newRWSetLoader        RWSetLoaderConstructor
	newCommitter          CommitterConstructor
	newDelivery           DeliveryConstructor
	newMembership         MembershipConstructor
	useFilteredDelivery   bool
}

func NewChannelProvider(
	envelopeKVS driver.EnvelopeStore,
	metadataKVS driver.MetadataStore,
	endorserTxKVS driver.EndorseTxStore,
	hasher hash.Hasher,
	drivers multiplexed.Driver,
	newVault VaultConstructor,
	channelConfigProvider driver.ChannelConfigProvider,
	newLedger LedgerConstructor,
	newRWSetLoader RWSetLoaderConstructor,
	newCommitter CommitterConstructor,
	newDelivery DeliveryConstructor,
	newMembership MembershipConstructor,
	useFilteredDelivery bool,
) *provider {
	return &provider{
		envelopeKVS:           envelopeKVS,
		metadataKVS:           metadataKVS,
		endorserTxKVS:         endorserTxKVS,
		hasher:                hasher,
		newVault:              newVault,
		drivers:               drivers,
		channelConfigProvider: channelConfigProvider,
		newLedger:             newLedger,
		newRWSetLoader:        newRWSetLoader,
		newCommitter:          newCommitter,
		newDelivery:           newDelivery,
		newMembership:         newMembership,
		useFilteredDelivery:   useFilteredDelivery,
	}
}

func (p *provider) NewChannel(nw driver.FabricNetworkService, channelName string, quiet bool) (driver.Channel, error) {
	// Channel configuration
	channelConfig, err := p.channelConfigProvider.GetChannelConfig(nw.Name(), channelName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting channel config for [%s]", channelName)
	}

	vaultStore, err := vault2.NewStore(nw.ConfigService().VaultPersistenceName(), p.drivers, nw.Name(), channelName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating vault store for channel [%s]", channelName)
	}

	vault, err := p.newVault(channelName, nw.ConfigService(), vaultStore)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating vault for channel [%s]", channelName)
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
		return nil, errors.Wrapf(err, "failed creating fabric finality for channel [%s]", channelName)
	}

	channelMembershipService := p.newMembership(channelName)

	// Committers
	rwSetLoaderService, err := p.newRWSetLoader(
		channelName,
		nw,
		envelopeService,
		transactionService,
		vault,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating RWSetLoader for channel [%s]", channelName)
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
		return nil, errors.Wrapf(err, "failed creating ledger for channel [%s]", channelName)
	}

	committerService, err := p.newCommitter(
		nw,
		channelName,
		vault,
		envelopeService,
		ledgerService,
		rwSetLoaderService,
		channelMembershipService,
		fabricFinality,
		quiet,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating committer for channel [%s]", channelName)
	}

	// Finality
	finalityService := committerService
	chaincodeManagerService.Finality = finalityService

	// Delivery
	deliveryService, err := p.newDelivery(
		nw,
		channelName,
		peerService,
		ledgerService,
		&vaultDeliveryWrapper{vaultStore: vaultStore},
		func(ctx context.Context, block *common.Block) (bool, error) {
			// commit the block, if an error occurs then retry
			return false, committerService.Commit(ctx, block)
		},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating delivery for channel [%s]", channelName)
	}

	c := &Channel{
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
		return nil, errors.Wrapf(err, "failed initializing Channel [%s]", channelName)
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
