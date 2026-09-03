/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// ValidationFlags is the per-transaction validation code vector carried in a
// block's TRANSACTIONS_FILTER metadata entry, one entry per transaction in
// block.Data.Data.
type ValidationFlags []uint8

// validationCodeAt safely returns the validation code for the transaction at
// index i in block. block.Data.Data and the TRANSACTIONS_FILTER metadata
// entry are populated independently by whoever produced the block, so a
// malformed or malicious block (delivered over the peer Deliver gRPC stream)
// can have a TRANSACTIONS_FILTER entry that is missing or shorter than
// Data.Data, which would otherwise panic on a bare slice index.
func validationCodeAt(block *common.Block, i int) (uint8, error) {
	if block.Metadata == nil || len(block.Metadata.Metadata) <= int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
		return 0, errors.Errorf("block [%d] metadata lacks transaction filter", block.GetHeader().GetNumber())
	}
	flags := ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])
	if i < 0 || i >= len(flags) {
		return 0, errors.Errorf("transaction index [%d] out of range for validation flags of length [%d] in block [%d]", i, len(flags), block.GetHeader().GetNumber())
	}
	return flags[i], nil
}

// Service is the channel-scoped entry point to the peer's Deliver service. It
// owns a long-running Delivery, started by Start, that feeds the committer, and
// it also serves one-off historical scans (ScanBlock, Scan and friends), each of
// which runs on its own short-lived Delivery.
type Service struct {
	channel             string
	channelConfig       driver.ChannelConfig
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

// NewService creates the delivery Service for a channel. It fails if
// channelConfig is nil, which would otherwise panic here: the wait-for-event
// timeout and buffer size are read off channelConfig while building the
// arguments to New.
func NewService(
	channel string,
	channelConfig driver.ChannelConfig,
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
	if channelConfig == nil {
		return nil, errors.New("expected channelConfig, got nil")
	}

	deliveryService, err := New(
		networkName,
		channelConfig,
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

// Start begins streaming blocks in the background. It returns as soon as the
// delivery goroutine is running, and never returns an error.
func (c *Service) Start(ctx context.Context) error {
	c.deliveryService.Start(ctx)
	return nil
}

// Stop shuts the background delivery down cleanly. It is idempotent and does
// not affect scans already in flight, which own their own Delivery.
func (c *Service) Stop() {
	c.deliveryService.Stop(nil)
}

// scanBlock runs a throwaway Delivery over this channel, starting from the
// position implied by vault, and invokes callback for each block. It blocks
// until the callback asks to stop, the callback fails, or ctx is cancelled.
func (c *Service) scanBlock(ctx context.Context, vault Vault, callback driver.BlockCallback) error {
	deliveryService, err := New(
		c.NetworkName,
		c.channelConfig,
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

// ScanBlock delivers whole blocks to callback, starting from the genesis block.
func (c *Service) ScanBlock(ctx context.Context, callback driver.BlockCallback) error {
	return c.scanBlock(ctx, &fakeVault{}, callback)
}

// ScanBlockFrom delivers whole blocks to callback, starting from the given
// block number.
func (c *Service) ScanBlockFrom(ctx context.Context, block driver.BlockNum, callback driver.BlockCallback) error {
	return c.scanBlock(ctx, &fakeVault{block: block}, callback)
}

// Scan delivers the transactions committed after txID to callback, one at a
// time and in block order, skipping any whose header type this Service was not
// configured to accept. Passing an empty txID starts from the genesis block.
func (c *Service) Scan(ctx context.Context, txID string, callback driver.DeliveryCallback) error {
	vault := &fakeVault{txID: txID}
	return c.scanBlock(ctx, vault,
		func(_ context.Context, block *common.Block) (bool, error) {
			for i, tx := range block.Data.Data {
				validationCode, err := validationCodeAt(block, i)
				if err != nil {
					logger.Errorf("[%s] %s", c.channel, err)
					return false, err
				}

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

// ScanFromBlock behaves like Scan but starts from the given block number
// instead of from a transaction ID.
func (c *Service) ScanFromBlock(ctx context.Context, block driver.BlockNum, callback driver.DeliveryCallback) error {
	vault := &fakeVault{block: block}
	return c.scanBlock(ctx, vault,
		func(_ context.Context, block *common.Block) (bool, error) {
			for i, tx := range block.Data.Data {
				validationCode, err := validationCodeAt(block, i)
				if err != nil {
					logger.Errorf("[%s] %s", c.channel, err)
					return false, err
				}

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

// processedTransaction is the driver.ProcessedTransaction handed to a
// DeliveryCallback during a scan.
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

// fakeVault is a stationary Vault used to seed a scan's start position. Unlike
// the real vault it is never advanced by the committer, so a scan always starts
// where the caller asked rather than where the node happens to have got to.
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
