/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func (c *committer) handleEndorserTransaction(block driver.Block, fBlock *pb.FilteredBlock, transactions []*pb.FilteredTransaction, i int, event *TxEvent) {
	tx := transactions[i]

	committer, err := c.network.Committer(c.channel)
	if err != nil {
		logger.Panicf("Cannot get Committer [%s]", err)
	}

	event.Txid = tx.Txid
	switch tx.TxValidationCode {
	case pb.TxValidationCode_VALID:
		logger.Debugf("transaction [%s] in fBlock [%d] is valid for fabric, commit!", tx.Txid, fBlock.Number)

		event.Committed = true
		event.Block = fBlock.Number
		event.IndexInBlock = i

		vc, deps, err := committer.Status(tx.Txid)
		if err != nil {
			logger.Panicf("failed getting tx's status [%s], with err [%s]", tx.Txid, err)
		}
		event.DependantTxIDs = append(event.DependantTxIDs, deps...)

		switch vc {
		case driver.Valid:
			logger.Debugf("transaction [%s] in fBlock [%d] is already marked as valid, skipping", tx.Txid, fBlock.Number)
			// Nothing to commit
			return
		case driver.Invalid:
			logger.Debugf("transaction [%s] in fBlock [%d] is marked as invalid, skipping", tx.Txid, fBlock.Number)
			// Nothing to commit
			return
		default:
			err = committer.CommitTX(event.Txid, event.Block, event.IndexInBlock, block.DataAt(i))
			if err != nil {
				logger.Panicf("failed committing transaction [%s] with deps [%v] with err [%s]", tx.Txid, deps, err)
			}
		}
	default:
		logger.Debugf("transaction [%s] in fBlock [%d] is not valid for fabric [%s], discard!", tx.Txid, fBlock.Number, tx.TxValidationCode)

		vc, deps, err := committer.Status(tx.Txid)
		if err != nil {
			logger.Panicf("failed getting tx's status [%s], with err [%s]", tx.Txid, err)
		}
		event.DependantTxIDs = append(event.DependantTxIDs, deps...)
		switch vc {
		case driver.Valid:
			// TODO: this might be due the fact that there are transactions with the same tx-id, the first is valid, the others are all invalid
			logger.Warnf("transaction [%s] in fBlock [%d] is marked as valid but for fabric is invalid", tx.Txid, fBlock.Number)
		case driver.Invalid:
			logger.Debugf("transaction [%s] in fBlock [%d] is marked as invalid, skipping", tx.Txid, fBlock.Number)
			// Nothing to commit
			return
		default:
			event.Err = errors.Errorf("transaction [%s] status is not valid: %s", tx.Txid, tx.TxValidationCode)
			err = committer.DiscardTx(event.Txid)
			if err != nil {
				logger.Errorf("failed discarding tx in state db with err [%s]", err)
			}
		}
	}
}
