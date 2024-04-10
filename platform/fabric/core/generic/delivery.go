/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"

	delivery2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

type ValidationFlags []uint8

type DeliveryService struct {
	channel             string
	channelConfig       driver.ChannelConfig
	hasher              hash.Hasher
	NetworkName         string
	LocalMembership     driver.LocalMembership
	ConfigService       driver.ConfigService
	PeerManager         delivery2.PeerManager
	Ledger              driver.Ledger
	waitForEventTimeout time.Duration

	deliveryService *delivery2.Delivery
}

func NewDeliveryService(
	channel string,
	channelConfig driver.ChannelConfig,
	hasher hash.Hasher,
	networkName string,
	localMembership driver.LocalMembership,
	configService driver.ConfigService,
	peerManager delivery2.PeerManager,
	ledger driver.Ledger,
	waitForEventTimeout time.Duration,
	txIDStore driver.TXIDStore,
	callback delivery2.Callback,
) (*DeliveryService, error) {
	deliveryService, err := delivery2.New(
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
	)
	if err != nil {
		return nil, err
	}

	return &DeliveryService{
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
	}, nil
}

func (c *DeliveryService) Start(ctx context.Context) error {
	c.deliveryService.Start(ctx)
	return nil
}

func (c *DeliveryService) Stop() {
	c.deliveryService.Stop()
}

func (c *DeliveryService) Scan(ctx context.Context, txID string, callback driver.DeliveryCallback) error {
	vault := &fakeVault{txID: txID}
	deliveryService, err := delivery2.New(
		c.NetworkName,
		c.channelConfig,
		c.hasher,
		c.LocalMembership,
		c.ConfigService,
		c.PeerManager,
		c.Ledger,
		func(block *common.Block) (bool, error) {
			for i, tx := range block.Data.Data {
				validationCode := ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])[i]

				if pb.TxValidationCode(validationCode) != pb.TxValidationCode_VALID {
					continue
				}

				env, err := protoutil.UnmarshalEnvelope(tx)
				if err != nil {
					logger.Errorf("Error getting tx from block: %s", err)
					return false, err
				}
				payload, err := protoutil.UnmarshalPayload(env.Payload)
				if err != nil {
					logger.Errorf("[%s] unmarshal payload failed: %s", c.channel, err)
					return false, err
				}
				channelHeader, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
				if err != nil {
					logger.Errorf("[%s] unmarshal Channel header failed: %s", c.channelConfig, err)
					return false, err
				}

				if common.HeaderType(channelHeader.Type) != common.HeaderType_ENDORSER_TRANSACTION {
					continue
				}

				ptx, err := transaction.NewProcessedTransactionFromEnvelopeRaw(tx)
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
		},
		vault,
		c.channelConfig.CommitterWaitForEventTimeout(),
	)
	if err != nil {
		return err
	}

	return deliveryService.Run(ctx)
}

type fakeVault struct {
	txID string
}

func (f *fakeVault) GetLastTxID() (string, error) {
	return f.txID, nil
}
