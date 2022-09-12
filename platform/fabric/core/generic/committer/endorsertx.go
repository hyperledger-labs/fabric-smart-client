/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type ValidationFlags []uint8

func (c *committer) handleEndorserTransaction(block *common.Block, i int, event *TxEvent, env *common.Envelope, chHdr *common.ChannelHeader) {
	txID := chHdr.TxId
	event.Txid = txID

	validationCode := pb.TxValidationCode(ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])[i])
	switch validationCode {
	case pb.TxValidationCode_VALID:

		if err := c.CommitEndorserTransaction(txID, block, i, env, event); err != nil {
			logger.Panicf("failed committing transaction [%s] with err [%s]", txID, err)
		}
		chaincodeEvent, err := getChaincodeEvent(env, block.Header.Number)
		if err != nil {
			logger.Panicf("Error reading chaincode event", err)
		}
		if chaincodeEvent != nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Chaincode Event Received: ", chaincodeEvent)
			}
			err := c.notifyChaincodeListeners(chaincodeEvent)
			if err != nil {
				logger.Panicf("Error sending chaincode events to listenerers")
			}
		}

	default:
		if err := c.DiscardEndorserTransaction(txID, block, event, validationCode); err != nil {
			logger.Panicf("failed discarding transaction [%s] with err [%s]", txID, err)
		}
	}
}

// CommitEndorserTransaction commits the transaction to the vault
func (c *committer) CommitEndorserTransaction(txID string, block *common.Block, indexInBlock int, env *common.Envelope, event *TxEvent) error {
	committer, err := c.network.Committer(c.channel)
	if err != nil {
		logger.Panicf("Cannot get Committer [%s]", err)
	}

	blockNum := block.Header.Number
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("transaction [%s] in block [%d] is valid for fabric, commit!", txID, blockNum)
	}

	event.Committed = true
	event.Block = blockNum
	event.IndexInBlock = indexInBlock

	vc, deps, err := committer.Status(txID)
	if err != nil {
		logger.Panicf("failed getting tx's status [%s], with err [%s]", txID, err)
	}
	event.DependantTxIDs = append(event.DependantTxIDs, deps...)

	switch vc {
	case driver.Valid:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("transaction [%s] in block [%d] is already marked as valid, skipping", txID, blockNum)
		}
		// Nothing to commit
	case driver.Invalid:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("transaction [%s] in block [%d] is marked as invalid, skipping", txID, blockNum)
		}
		// Nothing to commit
	default:
		if block != nil {
			if err := committer.CommitTX(event.Txid, event.Block, event.IndexInBlock, env); err != nil {
				logger.Panicf("failed committing transaction [%s] with deps [%v] with err [%s]", txID, deps, err)
			}
			return nil
		}

		if err := committer.CommitTX(event.Txid, event.Block, event.IndexInBlock, nil); err != nil {
			logger.Panicf("failed committing transaction [%s] with deps [%v] with err [%s]", txID, deps, err)
		}
	}
	return nil
}

// DiscardEndorserTransaction discards the transaction from the vault
func (c *committer) DiscardEndorserTransaction(txID string, block *common.Block, event *TxEvent, validationCode pb.TxValidationCode) error {
	committer, err := c.network.Committer(c.channel)
	if err != nil {
		logger.Panicf("Cannot get Committer [%s]", err)
	}

	blockNum := block.Header.Number
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("transaction [%s] in block [%d] is not valid for fabric [%s], discard!", txID, blockNum, validationCode)
	}

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
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("transaction [%s] in block [%d] is marked as invalid, skipping", txID, blockNum)
		}
		// Nothing to commit
	case driver.Unknown:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("transaction [%s] in block [%d] is marked as unknown, skipping", txID, blockNum)
		}
		// Nothing to commit
	default:
		event.Err = errors.Errorf("transaction [%s] status is not valid: %d", txID, validationCode)
		err = committer.DiscardTx(event.Txid)
		if err != nil {
			logger.Errorf("failed discarding tx in state db with err [%s]", err)
		}
	}
	return nil
}
