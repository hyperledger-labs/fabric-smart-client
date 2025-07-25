/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
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
	acceptedHeaderTypes collections.Set[common.HeaderType]
	tracerProvider      tracing.Provider
	metricsProvider     metrics.Provider
	deliveryService     *Delivery
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
	vault Vault,
	transactionManager driver.TransactionManager,
	callback driver.BlockCallback,
	tracerProvider tracing.Provider,
	metricsProvider metrics.Provider,
	acceptedHeaderTypes []common.HeaderType,
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
		vault,
		channelConfig.CommitterWaitForEventTimeout(),
		channelConfig.DeliveryBufferSize(),
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
		waitForEventTimeout: channelConfig.CommitterWaitForEventTimeout(),
		deliveryService:     deliveryService,
		transactionManager:  transactionManager,
		tracerProvider:      tracerProvider,
		metricsProvider:     metricsProvider,
		acceptedHeaderTypes: collections.NewSet(acceptedHeaderTypes...),
	}, nil
}

func (c *Service) Start(ctx context.Context) error {
	c.deliveryService.Start(ctx)
	return nil
}

func (c *Service) Stop() {
	c.deliveryService.Stop(nil)
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
		c.channelConfig.DeliveryBufferSize(),
		c.tracerProvider,
		c.metricsProvider,
	)
	if err != nil {
		return err
	}

	return deliveryService.Run(ctx)
}

func (c *Service) ScanBlock(ctx context.Context, callback driver.BlockCallback) error {
	return c.scanBlock(ctx, &fakeVault{}, callback)
}

func (c *Service) ScanBlockFrom(ctx context.Context, block driver.BlockNum, callback driver.BlockCallback) error {
	return c.scanBlock(ctx, &fakeVault{block: block}, callback)
}

func (c *Service) Scan(ctx context.Context, txID string, callback driver.DeliveryCallback) error {
	vault := &fakeVault{txID: txID}
	return c.scanBlock(ctx, vault,
		func(_ context.Context, block *common.Block) (bool, error) {
			for i, tx := range block.Data.Data {
				validationCode := ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])[i]

				// if pb.TxValidationCode(validationCode) != pb.TxValidationCode_VALID {
				//	continue
				// }
				_, _, channelHeader, err := fabricutils.UnmarshalTx(tx)
				if err != nil {
					logger.Errorf("[%s] unmarshal tx failed: %s", c.channel, err)
					return false, err
				}

				if !c.acceptedHeaderTypes.Contains(common.HeaderType(channelHeader.Type)) {
					continue
				}
				ptx, err := c.transactionManager.NewProcessedTransactionFromEnvelopeRaw(tx)
				if err != nil {
					return false, err
				}

				stop, err := callback(&processedTransaction{
					txID:    ptx.TxID(),
					results: ptx.Results(),
					vc:      int32(validationCode),
					env:     ptx.Envelope(),
				})
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

func (c *Service) ScanFromBlock(ctx context.Context, block driver.BlockNum, callback driver.DeliveryCallback) error {
	vault := &fakeVault{block: block}
	return c.scanBlock(ctx, vault,
		func(_ context.Context, block *common.Block) (bool, error) {
			for i, tx := range block.Data.Data {
				validationCode := ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])[i]

				// if pb.TxValidationCode(validationCode) != pb.TxValidationCode_VALID {
				//	continue
				// }
				_, _, channelHeader, err := fabricutils.UnmarshalTx(tx)
				if err != nil {
					logger.Errorf("[%s] unmarshal tx failed: %s", c.channel, err)
					return false, err
				}

				if !c.acceptedHeaderTypes.Contains(common.HeaderType(channelHeader.Type)) {
					continue
				}
				ptx, err := c.transactionManager.NewProcessedTransactionFromEnvelopeRaw(tx)
				if err != nil {
					return false, err
				}

				stop, err := callback(&processedTransaction{
					txID:    ptx.TxID(),
					results: ptx.Results(),
					vc:      int32(validationCode),
					env:     ptx.Envelope(),
				})
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

type processedTransaction struct {
	txID    driver.TxID
	results []byte
	vc      int32
	env     []byte
}

func (p *processedTransaction) TxID() string {
	return p.txID
}

func (p *processedTransaction) Results() []byte {
	return p.results
}

func (p *processedTransaction) IsValid() bool {
	return p.vc == int32(pb.TxValidationCode_VALID)
}

func (p *processedTransaction) Envelope() []byte {
	return p.env
}

func (p *processedTransaction) ValidationCode() int32 {
	return p.vc
}

type fakeVault struct {
	txID  driver.TxID
	block driver.BlockNum
}

func (f *fakeVault) GetLastTxID(context.Context) (string, error) {
	return f.txID, nil
}

func (f *fakeVault) GetLastBlock(context.Context) (uint64, error) {
	return f.block, nil
}
