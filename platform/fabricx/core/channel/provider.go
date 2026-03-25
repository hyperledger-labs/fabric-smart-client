/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering"
	genericservices "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	channelconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
)

var logger = logging.MustGetLogger()

type VaultConstructor = func(
	channelName string,
	configService fdriver.ConfigService,
	vaultStore cdriver.VaultStore,
) (*vault.Vault, error)

type LedgerConstructor func(
	channelName string,
	nw fdriver.FabricNetworkService,
	chaincodeManager fdriver.ChaincodeManager,
) (fdriver.Ledger, error)

type RWSetLoaderConstructor func(
	channel string,
	nw fdriver.FabricNetworkService,
	envelopeService fdriver.EnvelopeService,
	transactionService fdriver.EndorserTransactionService,
	vault fdriver.RWSetInspector,
) (fdriver.RWSetLoader, error)

type DeliveryConstructor func(
	nw fdriver.FabricNetworkService,
	channel string,
	peerManager delivery.Services,
	ledger fdriver.Ledger,
	vault delivery.Vault,
	callback fdriver.BlockCallback,
) (generic.DeliveryService, error)

type MembershipConstructor func(channelName string) fdriver.MembershipService

type ChannelProvider interface {
	NewChannel(nw fdriver.FabricNetworkService, name string, quiet bool) (fdriver.Channel, error)
}

type LedgerProvider interface {
	NewLedger(network, channel string) (fdriver.Ledger, error)
}

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
	deliveryService, err := p.newDelivery(nw, channelName, peerService, ledgerService, &vaultDeliveryWrapper{vaultStore: vaultStore}, nil)
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
		VaultStoreService:        vaultStore,
		ES:                       envelopeService,
		TS:                       transactionService,
		MS:                       metadataService,
		DeliveryService:          &nopeDeliveryService{DeliveryService: deliveryService},
		RWSetLoaderService:       rwSetLoaderService,
		LedgerService:            ledgerService,
		ChannelMembershipService: channelMembershipService,
		ChaincodeManagerService:  nil,
		CommitterService:         &nopeCommitterService{},
	}

	if err := startChannelConfigMonitor(nw, channelName, channelMembershipService, p.queryServiceProvider); err != nil {
		return nil, errors.Wrapf(err, "failed starting channel config monitor for channel [%s]", channelName)
	}

	return c, nil
}

// finalityServiceAdapter adapts finality.ListenerManager to implement driver.Finality
type finalityServiceAdapter struct {
	manager finality.ListenerManager
}

// IsFinal implements the driver.Finality interface by registering a listener
// and waiting for the transaction to reach finality
func (f *finalityServiceAdapter) IsFinal(ctx context.Context, txID string) error {
	// Create a channel to receive the finality notification
	done := make(chan struct {
		status int
		err    error
	}, 1)

	// Create a listener that will be called when the transaction reaches finality
	listener := &finalityListener{
		onStatusFunc: func(ctx context.Context, txID string, status int, statusMessage string) {
			var err error
			switch status {
			case fdriver.Valid:
				// Transaction is valid and committed - success
				err = nil
			case fdriver.Invalid:
				// Transaction is invalid
				err = errors.Errorf("transaction [%s] is invalid: %s", txID, statusMessage)
			case fdriver.Unknown:
				// Transaction status is unknown (e.g., timeout)
				err = errors.Errorf("transaction [%s] status is unknown: %s", txID, statusMessage)
			default:
				// Unexpected status
				err = errors.Errorf("transaction [%s] has unexpected status %d: %s", txID, status, statusMessage)
			}

			select {
			case done <- struct {
				status int
				err    error
			}{status: status, err: err}:
			default:
				// Channel already has a value, ignore
			}
		},
	}

	// Register the listener
	if err := f.manager.AddFinalityListener(txID, listener); err != nil {
		return errors.Wrapf(err, "failed to add finality listener for transaction [%s]", txID)
	}

	// Ensure cleanup: remove the listener when we're done
	defer func() {
		if err := f.manager.RemoveFinalityListener(txID, listener); err != nil {
			logger.Warnf("failed to remove finality listener for transaction [%s]: %v", txID, err)
		}
	}()

	// Wait for either the finality notification or context cancellation
	select {
	case result := <-done:
		return result.err
	case <-ctx.Done():
		return errors.Wrapf(ctx.Err(), "context cancelled while waiting for transaction [%s] finality", txID)
	}
}

// finalityListener implements the fabric.FinalityListener interface
type finalityListener struct {
	onStatusFunc func(ctx context.Context, txID string, status int, statusMessage string)
	mu           sync.Mutex
}

func (l *finalityListener) OnStatus(ctx context.Context, txID string, status int, statusMessage string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.onStatusFunc != nil {
		l.onStatusFunc(ctx, txID, status, statusMessage)
	}
}

type vaultDeliveryWrapper struct {
	vaultStore cdriver.VaultStore
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

// startChannelConfigMonitor creates and starts the channel configuration monitor
func startChannelConfigMonitor(
	nw fdriver.FabricNetworkService,
	channel string,
	channelMembershipService fdriver.MembershipService,
	qsProvider queryservice.Provider,
) error {
	// Get query service for this channel
	qs, err := qsProvider.Get(nw.Name(), channel)
	if err != nil {
		return errors.Wrapf(err, "failed to get query service for channel [%s]", channel)
	}

	// Create channel config monitor configuration
	monitorConfig, err := channelconfig.NewConfig(nw.ConfigService(), nw.Name(), channel)
	if err != nil {
		return errors.Wrapf(err, "failed to create channel config monitor config for channel [%s]", channel)
	}

	// Create adapters for the interfaces
	orderingServiceAdapter := &orderingServiceAdapter{os: nw.OrderingService()}

	// Create the monitor with the required dependencies
	monitor, err := channelconfig.NewChannelConfigMonitor(
		monitorConfig,
		qs,
		channelMembershipService,
		orderingServiceAdapter,
		nw.ConfigService(),
		nw.Name(),
		channel,
	)
	if err != nil {
		return errors.Wrapf(err, "failed to create channel config monitor for channel [%s]", channel)
	}

	// Start the monitor
	if err := monitor.Start(context.Background()); err != nil {
		return errors.Wrapf(err, "failed to start channel config monitor for channel [%s]", channel)
	}

	return nil
}

// orderingServiceAdapter adapts fdriver.Ordering to config.OrderingService
type orderingServiceAdapter struct {
	os fdriver.Ordering
}

func (a *orderingServiceAdapter) Configure(consensusType string, orderers []*grpc.ConnectionConfig) error {
	// Cast to *ordering.Service which has the Configure method
	orderingService, ok := a.os.(*ordering.Service)
	if !ok {
		return errors.New("ordering service is not an *ordering.Service")
	}
	return orderingService.Configure(consensusType, orderers)
}

type nopeCommitterService struct{}

func (n *nopeCommitterService) IsFinal(ctx context.Context, txID string) error {
	return nil
}

func (n *nopeCommitterService) ReloadConfigTransactions() error {
	return nil
}

func (n *nopeCommitterService) Commit(ctx context.Context, block *common.Block) error {
	return nil
}

func (n *nopeCommitterService) Start(context context.Context) error {
	return nil
}

func (n *nopeCommitterService) ProcessNamespace(nss ...cdriver.Namespace) error {
	return nil
}

func (n *nopeCommitterService) AddTransactionFilter(tf fdriver.TransactionFilter) error {
	return nil
}

func (n *nopeCommitterService) Status(context context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
	return 0, "", nil
}

func (n *nopeCommitterService) AddFinalityListener(txID string, listener fdriver.FinalityListener) error {
	return nil
}

func (n *nopeCommitterService) RemoveFinalityListener(txID string, listener fdriver.FinalityListener) error {
	return nil
}

func (n *nopeCommitterService) DiscardTx(context context.Context, txID cdriver.TxID, message string) error {
	return nil
}

func (n *nopeCommitterService) CommitTX(ctx context.Context, txID cdriver.TxID, block cdriver.BlockNum, indexInBlock cdriver.TxNum, envelope *common.Envelope) error {
	return nil
}

type nopeDeliveryService struct {
	generic.DeliveryService
}

func (n *nopeDeliveryService) Start(ctx context.Context) error {
	return nil
}
