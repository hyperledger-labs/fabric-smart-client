/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	delivery2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

type ValidationFlags []uint8

func (c *channel) StartDelivery(ctx context.Context) error {
	c.deliveryService.Start(ctx)
	return nil
}

func (c *channel) Scan(ctx context.Context, txID string, callback driver.DeliveryCallback) error {
	vault := &fakeVault{txID: txID}
	deliveryService, err := delivery2.New(c.name, c.sp, c.network, func(block *common.Block) (bool, error) {
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
				logger.Errorf("[%s] unmarshal payload failed: %s", c.name, err)
				return false, err
			}
			channelHeader, err := protoutil.UnmarshalChannelHeader(payload.Header.ChannelHeader)
			if err != nil {
				logger.Errorf("[%s] unmarshal channel header failed: %s", c.name, err)
				return false, err
			}

			if common.HeaderType(channelHeader.Type) != common.HeaderType_ENDORSER_TRANSACTION {
				continue
			}

			ptx, err := newProcessedTransactionFromEnvelopeRaw(tx)
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
	}, vault, waitForEventTimeout)
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
