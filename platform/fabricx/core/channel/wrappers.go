/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	channelconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
)

type VaultConstructor = func(
	channelName string,
	configService fdriver.ConfigService,
	vaultStore cdriver.VaultStore,
) (fdriver.Vault, error)

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
}

func (l *finalityListener) OnStatus(ctx context.Context, txID string, status int, statusMessage string) {
	l.onStatusFunc(ctx, txID, status, statusMessage)
}

type fakeVault struct{}

func (f *fakeVault) GetLastTxID(ctx context.Context) (string, error) {
	return "", nil
}

func (f *fakeVault) GetLastBlock(context.Context) (uint64, error) {
	return 0, nil
}

// startChannelConfigMonitor creates and starts the channel configuration monitor
func startChannelConfigMonitor(nw fdriver.FabricNetworkService, channel string, channelMembershipService fdriver.MembershipService, qsProvider queryservice.Provider) (*channelconfig.ChannelConfigMonitor, error) {
	// Get query service for this channel
	qs, err := qsProvider.Get(nw.Name(), channel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get query service for channel [%s]", channel)
	}

	monitorConfig, err := channelconfig.NewConfig(nw.ConfigService())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create channel config monitor config for channel [%s]", channel)
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
		return nil, errors.Wrapf(err, "failed to create channel config monitor for channel [%s]", channel)
	}

	// Start the monitor
	if err := monitor.Start(context.Background()); err != nil {
		return nil, errors.Wrapf(err, "failed to start channel config monitor for channel [%s]", channel)
	}

	return monitor, nil
}

// orderingServiceAdapter adapts fdriver.Ordering to config.OrderingService
type orderingServiceAdapter struct {
	os fdriver.Ordering
}

func (a *orderingServiceAdapter) Configure(consensusType string, orderers []*grpc.ConnectionConfig) error {
	// Cast to *ordering.ChannelConfigMonitor which has the Configure method
	orderingService, ok := a.os.(*ordering.Service)
	if !ok {
		return errors.New("ordering service is not an *ordering.ChannelConfigMonitor")
	}
	return orderingService.Configure(consensusType, orderers)
}

// committerService implements the driver.Committer interface to offer only the finality service.
// The rest is not implemented because not needed by fabric-x
type committerService struct {
	finalityService fdriver.Finality
}

func (n *committerService) IsFinal(ctx context.Context, txID string) error {
	if n.finalityService != nil {
		return n.finalityService.IsFinal(ctx, txID)
	}
	return nil
}

func (n *committerService) ReloadConfigTransactions() error {
	return nil
}

func (n *committerService) Commit(_ context.Context, block *common.Block) error {
	return nil
}

func (n *committerService) Start(_ context.Context) error {
	return nil
}

func (n *committerService) ProcessNamespace(nss ...cdriver.Namespace) error {
	return nil
}

func (n *committerService) AddTransactionFilter(tf fdriver.TransactionFilter) error {
	return nil
}

func (n *committerService) Status(_ context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
	return 0, "", nil
}

func (n *committerService) AddFinalityListener(txID string, listener fdriver.FinalityListener) error {
	if n.finalityService != nil {
		if lm, ok := n.finalityService.(*finalityServiceAdapter); ok {
			return lm.manager.AddFinalityListener(txID, listener)
		}
	}
	return nil
}

func (n *committerService) RemoveFinalityListener(txID string, listener fdriver.FinalityListener) error {
	if n.finalityService != nil {
		if lm, ok := n.finalityService.(*finalityServiceAdapter); ok {
			return lm.manager.RemoveFinalityListener(txID, listener)
		}
	}
	return nil
}

func (n *committerService) DiscardTx(_ context.Context, txID cdriver.TxID, message string) error {
	return nil
}

func (n *committerService) CommitTX(_ context.Context, txID cdriver.TxID, block cdriver.BlockNum, indexInBlock cdriver.TxNum, envelope *common.Envelope) error {
	return nil
}

type noopDeliveryService struct {
	generic.DeliveryService
}

func (n *noopDeliveryService) Start(_ context.Context) error {
	return nil
}
