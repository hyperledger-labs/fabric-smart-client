/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type ValidationFlags []uint8

type Service struct {
	channel             string
	channelConfig       driver.ChannelConfig
	hasher              hash.Hasher
	NetworkName         string
	LocalMembership     driver.LocalMembership
	ConfigService       driver.ConfigService
	PeerManager         Services
	Ledger              driver.Ledger
	transactionManager  driver.TransactionManager
	waitForEventTimeout time.Duration

	deliveryService *Delivery
}

func NewService(
	channel string,
	channelConfig driver.ChannelConfig,
	hasher hash.Hasher,
	networkName string,
	localMembership driver.LocalMembership,
	configService driver.ConfigService,
	peerManager Services,
	ledger driver.Ledger,
	waitForEventTimeout time.Duration,
	txIDStore driver.TXIDStore,
	transactionManager driver.TransactionManager,
	callback driver.BlockCallback,
	tracerProvider trace.TracerProvider,
	metricsProvider metrics.Provider,
) (*Service, error) {
	deliveryService, err := New(
		networkName,
		channelConfig,
		hasher,
		localMembership,
		configService,
		peerManager,
		ledger,
		callback,
		txIDStore,
		channelConfig.CommitterWaitForEventTimeout(),
		tracerProvider,
		metricsProvider,
	)
	if err != nil {
		return nil, err
	}

	return &Service{
		channel:             channel,
		channelConfig:       channelConfig,
		hasher:              hasher,
		NetworkName:         networkName,
		LocalMembership:     localMembership,
		ConfigService:       configService,
		PeerManager:         peerManager,
		Ledger:              ledger,
		waitForEventTimeout: waitForEventTimeout,
		deliveryService:     deliveryService,
		transactionManager:  transactionManager,
	}, nil
}

func (c *Service) Start(ctx context.Context) error {
	c.deliveryService.Start(ctx)
	return nil
}

func (c *Service) Stop() {
	c.deliveryService.Stop()
}

func (c *Service) scanBlock(ctx context.Context, vault Vault, callback driver.BlockCallback) error {
	deliveryService, err := New(
		c.NetworkName,
		c.channelConfig,
		c.hasher,
		c.LocalMembership,
		c.ConfigService,
		c.PeerManager,
		c.Ledger,
		callback,
		vault,
		c.channelConfig.CommitterWaitForEventTimeout(),
		&noop.TracerProvider{},
		&disabled.Provider{},
	)
	if err != nil {
		return err
	}

	return deliveryService.Run(ctx)
}

func (c *Service) ScanBlock(ctx context.Context, callback driver.BlockCallback) error {
	return c.scanBlock(ctx, &fakeVault{}, callback)
}

func (c *Service) Scan(ctx context.Context, txID string, callback driver.DeliveryCallback) error {
	vault := &fakeVault{txID: txID}
	return c.scanBlock(ctx, vault,
		func(_ context.Context, block *common.Block) (bool, error) {
			for i, tx := range block.Data.Data {
				validationCode := ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])[i]

				if pb.TxValidationCode(validationCode) != pb.TxValidationCode_VALID {
					continue
				}
				_, _, channelHeader, err := fabricutils.UnmarshalTx(tx)
				if err != nil {
					logger.Errorf("[%s] unmarshal tx failed: %s", c.channel, err)
					return false, err
				}

				if common.HeaderType(channelHeader.Type) != common.HeaderType_ENDORSER_TRANSACTION {
					continue
				}

				ptx, err := c.transactionManager.NewProcessedTransactionFromEnvelopeRaw(tx)
				if err != nil {
					return false, err
				}

				stop, err := callback(ptx)
				if err != nil {
					// if an error occurred, stop processing
					return false, err
				}
				if stop {
					return true, nil
				}
				vault.txID = channelHeader.TxId
				logger.Debugf("commit transaction [%s] in block [%d]", channelHeader.TxId, block.Header.Number)
			}
			return false, nil
		})
}

type fakeVault struct {
	txID string
}

func (f *fakeVault) GetLastTxID() (string, error) {
	return f.txID, nil
}
