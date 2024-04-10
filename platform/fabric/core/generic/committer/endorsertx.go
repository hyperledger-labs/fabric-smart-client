/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

type ValidationFlags []uint8

func (c *Committer) HandleEndorserTransaction(block *common.Block, i int, event *TxEvent, env *common.Envelope, chHdr *common.ChannelHeader) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("[%s] Endorser transaction received: %s", c.Channel, chHdr.TxId)
	}
	if len(block.Metadata.Metadata) < int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
		return errors.Errorf("block metadata lacks transaction filter")
	}

	txID := chHdr.TxId
	event.TxID = txID
	event.ValidationCode = pb.TxValidationCode(ValidationFlags(block.Metadata.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])[i])
	event.ValidationMessage = pb.TxValidationCode_name[int32(event.ValidationCode)]

	switch event.ValidationCode {
	case pb.TxValidationCode_VALID:
		processed, err := c.CommitEndorserTransaction(txID, block, i, env, event)
		if err != nil {
			if errors2.HasCause(err, ErrDiscardTX) {
				// in this case, we will discard the transaction
				event.ValidationCode = pb.TxValidationCode_INVALID_OTHER_REASON
				event.ValidationMessage = err.Error()
				break
			}
			return errors.Wrapf(err, "failed committing transaction [%s]", txID)
		}
		if !processed {
			if err := c.GetChaincodeEvents(env, block); err != nil {
				return errors.Wrapf(err, "failed to publish chaincode events [%s]", txID)
			}
		}
		return nil
	}

	if err := c.DiscardEndorserTransaction(txID, block, event); err != nil {
		return errors.Wrapf(err, "failed discarding transaction [%s]", txID)
	}
	return nil
}

// GetChaincodeEvents reads the chaincode events and notifies the listeners registered to the specific chaincode.
func (c *Committer) GetChaincodeEvents(env *common.Envelope, block *common.Block) error {
	chaincodeEvent, err := readChaincodeEvent(env, block.Header.Number)
	if err != nil {
		return errors.Wrapf(err, "error reading chaincode event")
	}
	if chaincodeEvent != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("Chaincode Event Received: ", chaincodeEvent)
		}
		c.notifyChaincodeListeners(chaincodeEvent)
	}
	return nil
}

// CommitEndorserTransaction commits the transaction to the vault.
// It returns true, if the transaction was already processed, false otherwise.
func (c *Committer) CommitEndorserTransaction(txID string, block *common.Block, indexInBlock int, env *common.Envelope, event *TxEvent) (bool, error) {
	committer, err := c.Network.Committer(c.Channel)
	if err != nil {
		return false, errors.Wrapf(err, "cannot get Committer for channel [%s]", c.Channel)
	}

	blockNum := block.Header.Number
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("transaction [%s] in block [%d] is valid for fabric, commit!", txID, blockNum)
	}

	event.Committed = true
	event.Block = blockNum
	event.IndexInBlock = indexInBlock

	vc, _, deps, err := committer.Status(txID)
	if err != nil {
		return false, errors.Wrapf(err, "failed getting tx's status [%s]", txID)
	}
	event.DependantTxIDs = append(event.DependantTxIDs, deps...)

	switch vc {
	case driver.Valid:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("transaction [%s] in block [%d] is already marked as valid, skipping", txID, blockNum)
		}
		// Nothing to commit
		return true, nil
	case driver.Invalid:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("transaction [%s] in block [%d] is marked as invalid, skipping", txID, blockNum)
		}
		// Nothing to commit
		return true, nil
	}

	if block != nil {
		if err := committer.CommitTX(event.TxID, event.Block, event.IndexInBlock, env); err != nil {
			return false, errors.Wrapf(err, "failed committing transaction [%s] with deps [%v]", txID, deps)
		}
		return false, nil
	}
	if err := committer.CommitTX(event.TxID, event.Block, event.IndexInBlock, nil); err != nil {
		return false, errors.Wrapf(err, "failed committing transaction [%s] with deps [%v]", txID, deps)
	}
	return false, nil
}

// DiscardEndorserTransaction discards the transaction from the vault
func (c *Committer) DiscardEndorserTransaction(txID string, block *common.Block, event *TxEvent) error {
	committer, err := c.Network.Committer(c.Channel)
	if err != nil {
		return errors.Wrapf(err, "cannot get Committer for channel [%s]", c.Channel)
	}

	blockNum := block.Header.Number
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("transaction [%s] in block [%d] is not valid for fabric [%s], discard!", txID, blockNum, event.ValidationCode)
	}

	vc, _, deps, err := committer.Status(txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txID)
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
	default:
		event.Err = errors.Errorf("transaction [%s] status is not valid [%d], message [%s]", txID, event.ValidationCode, event.ValidationMessage)
		err = committer.DiscardTx(event.TxID, event.ValidationMessage)
		if err != nil {
			logger.Errorf("failed discarding tx in state db with err [%s]", err)
		}
	}

	return nil
}
