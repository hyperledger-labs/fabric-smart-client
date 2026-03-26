/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	genericservices "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	channelconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
)

var logger = logging.MustGetLogger()

type provider struct {
	configProvider          config.Provider
	envelopeKVS             fdriver.EnvelopeStore
	metadataKVS             fdriver.MetadataStore
	endorserTxKVS           fdriver.EndorseTxStore
	newVault                VaultConstructor
	drivers                 multiplexed.Driver
	channelConfigProvider   fdriver.ChannelConfigProvider
	newLedger               LedgerConstructor
	newRWSetLoader          RWSetLoaderConstructor
	newDelivery             DeliveryConstructor
	newMembership           MembershipConstructor
	useFilteredDelivery     bool
	queryServiceProvider    queryservice.Provider
	listenerManagerProvider finality.ListenerManagerProvider
}

func NewProvider(
	configProvider config.Provider,
	envelopeKVS fdriver.EnvelopeStore,
	metadataKVS fdriver.MetadataStore,
	endorserTxKVS fdriver.EndorseTxStore,
	drivers multiplexed.Driver,
	newVault VaultConstructor,
	channelConfigProvider fdriver.ChannelConfigProvider,
	newLedger LedgerConstructor,
	newRWSetLoader RWSetLoaderConstructor,
	newDelivery DeliveryConstructor,
	newMembership MembershipConstructor,
	useFilteredDelivery bool,
	queryServiceProvider queryservice.Provider,
	listenerManagerProvider finality.ListenerManagerProvider,
) *provider {
	return &provider{
		configProvider:          configProvider,
		envelopeKVS:             envelopeKVS,
		metadataKVS:             metadataKVS,
		endorserTxKVS:           endorserTxKVS,
		newVault:                newVault,
		drivers:                 drivers,
		channelConfigProvider:   channelConfigProvider,
		newLedger:               newLedger,
		newRWSetLoader:          newRWSetLoader,
		newDelivery:             newDelivery,
		newMembership:           newMembership,
		useFilteredDelivery:     useFilteredDelivery,
		queryServiceProvider:    queryServiceProvider,
		listenerManagerProvider: listenerManagerProvider,
	}
}

func (p *provider) NewChannel(nw fdriver.FabricNetworkService, channelName string, quiet bool) (fdriver.Channel, error) {
	vault, err := p.newVault(channelName, nw.ConfigService(), nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating vault for channel [%s]", channelName)
	}
	envelopeService := transaction.NewEnvelopeService(p.envelopeKVS, nw.Name(), channelName)
	transactionService := transaction.NewEndorseTransactionService(p.endorserTxKVS, nw.Name(), channelName)
	metadataService := transaction.NewMetadataService(p.metadataKVS, nw.Name(), channelName)
	peerService := genericservices.NewClientFactory(nw.ConfigService(), nw.LocalMembership().DefaultSigningIdentity())

	channelMembershipService := p.newMembership(channelName)

	// Committers
	rwSetLoaderService, err := p.newRWSetLoader(channelName, nw, envelopeService, transactionService, vault)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating RWSetLoader for channel [%s]", channelName)
	}

	ledgerService, err := p.newLedger(channelName, nw, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating ledger for channel [%s]", channelName)
	}

	// Delivery
	deliveryService, err := p.newDelivery(nw, channelName, peerService, ledgerService, &fakeVault{}, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating delivery for channel [%s]", channelName)
	}

	// Create finality service using the listener manager provider
	listenerManager, err := p.listenerManagerProvider.NewManager(nw.Name(), channelName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating listener manager for channel [%s]", channelName)
	}
	finalityService := &finalityServiceAdapter{manager: listenerManager}

	c := &generic.Channel{
		ChannelName:              channelName,
		FinalityService:          finalityService,
		VaultService:             vault,
		ES:                       envelopeService,
		TS:                       transactionService,
		MS:                       metadataService,
		DeliveryService:          &noopDeliveryService{DeliveryService: deliveryService},
		RWSetLoaderService:       rwSetLoaderService,
		LedgerService:            ledgerService,
		ChannelMembershipService: channelMembershipService,
		ChaincodeManagerService:  nil,
		CommitterService:         &committerService{finalityService: finalityService},
	}

	monitor, err := startChannelConfigMonitor(nw, channelName, channelMembershipService, p.queryServiceProvider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed starting channel config monitor for channel [%s]", channelName)
	}

	return &channel{
		Channel: c,
		Monitor: monitor,
	}, nil
}

type channel struct {
	fdriver.Channel
	Monitor *channelconfig.ChannelConfigMonitor
}

func (c *channel) Close() error {
	return errors.Join(
		c.Channel.Close(),
		c.Monitor.Stop(),
	)
}
