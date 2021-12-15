/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type ValidationFlags []uint8

func (c *committer) handleEndorserTransaction(block *common.Block, i int, event *TxEvent, chHdr *common.ChannelHeader) {
	committer, err := c.network.Committer(c.channel)
	if err != nil {
		logger.Panicf("Cannot get Committer [%s]", err)
	}

	txID := chHdr.TxId
	event.Txid = txID

	validationCode := ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])[i]
	blockNum := block.Header.Number
	switch pb.TxValidationCode(validationCode) {
	case pb.TxValidationCode_VALID:
		logger.Debugf("transaction [%s] in block [%d] is valid for fabric, commit!", txID, blockNum)

		event.Committed = true
		event.Block = blockNum
		event.IndexInBlock = i

		vc, deps, err := committer.Status(txID)
		if err != nil {
			logger.Panicf("failed getting tx's status [%s], with err [%s]", txID, err)
		}
		event.DependantTxIDs = append(event.DependantTxIDs, deps...)

		switch vc {
		case driver.Valid:
			logger.Debugf("transaction [%s] in block [%d] is already marked as valid, skipping", txID, blockNum)
			// Nothing to commit
			return
		case driver.Invalid:
			logger.Debugf("transaction [%s] in block [%d] is marked as invalid, skipping", txID, blockNum)
			// Nothing to commit
			return
		default:
			if block != nil {
				if err := committer.CommitTX(event.Txid, event.Block, event.IndexInBlock, block.Data.Data[i]); err != nil {
					logger.Panicf("failed committing transaction [%s] with deps [%v] with err [%s]", txID, deps, err)
				}
				return
			}

			if err := committer.CommitTX(event.Txid, event.Block, event.IndexInBlock, nil); err != nil {
				logger.Panicf("failed committing transaction [%s] with deps [%v] with err [%s]", txID, deps, err)
			}
		}
	default:
		logger.Debugf("transaction [%s] in block [%d] is not valid for fabric [%s], discard!", txID, blockNum, validationCode)

		vc, deps, err := committer.Status(txID)
		if err != nil {
			logger.Panicf("failed getting tx's status [%s], with err [%s]", txID, err)
		}
		event.DependantTxIDs = append(event.DependantTxIDs, deps...)
		switch vc {
		case driver.Valid:
			// TODO: this might be due the fact that there are transactions with the same tx-id, the first is valid, the others are all invalid
			logger.Warnf("transaction [%s] in block [%d] is marked as valid but for fabric is invalid", txID, blockNum)
		case driver.Invalid:
			logger.Debugf("transaction [%s] in block [%d] is marked as invalid, skipping", txID, blockNum)
			// Nothing to commit
			return
		default:
			event.Err = errors.Errorf("transaction [%s] status is not valid: %d", txID, validationCode)
			err = committer.DiscardTx(event.Txid)
			if err != nil {
				logger.Errorf("failed discarding tx in state db with err [%s]", err)
			}
		}
	}
}
