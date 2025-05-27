/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/pkg/errors"
)

type ValidationFlags []uint8

func (c *Committer) HandleEndorserTransaction(ctx context.Context, block *common.BlockMetadata, tx CommitTx) (*FinalityEvent, error) {
	logger.DebugfContext(ctx, "[%s] EndorserClient transaction received: %s", c.ChannelConfig.ID(), tx.TxID)
	fabricValidationCode, event, err := MapFinalityEvent(ctx, block, tx.TxNum, tx.TxID)
	if err != nil {
		return nil, err
	}

	switch pb.TxValidationCode(fabricValidationCode) {
	case pb.TxValidationCode_VALID:
		processed, err := c.CommitEndorserTransaction(ctx, event.TxID, tx.BlkNum, tx.TxNum, tx.Envelope, event)
		if err != nil {
			if errors2.HasCause(err, ErrDiscardTX) {
				// in this case, we will discard the transaction
				event.ValidationCode = convertValidationCode(int32(pb.TxValidationCode_INVALID_OTHER_REASON))
				event.ValidationMessage = err.Error()
				break
			}
			return nil, errors.Wrapf(err, "failed committing transaction [%s]", event.TxID)
		}
		if !processed {
			if err := c.GetChaincodeEvents(tx.Envelope, tx.BlkNum); err != nil {
				return nil, errors.Wrapf(err, "failed to publish chaincode events [%s]", event.TxID)
			}
		}
		c.logger.DebugfContext(ctx, "Processed transaction")
		return event, nil
	}

	if err := c.DiscardEndorserTransaction(ctx, event.TxID, tx.BlkNum, tx.Raw, event); err != nil {
		return nil, errors.Wrapf(err, "failed discarding transaction [%s]", event.TxID)
	}
	c.logger.DebugfContext(ctx, "Discarded transaction")
	return event, nil
}

func MapFinalityEvent(ctx context.Context, block *common.BlockMetadata, txNum driver.TxNum, txID string) (uint8, *FinalityEvent, error) {
	if len(block.Metadata) < int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
		return 0, nil, errors.Errorf("block metadata lacks transaction filter")
	}

	fabricValidationCode := ValidationFlags(block.Metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER])[txNum]
	validationCode, validationMessage := MapValidationCode(int32(fabricValidationCode))
	event := &FinalityEvent{
		Ctx:               ctx,
		TxID:              txID,
		ValidationCode:    validationCode,
		ValidationMessage: validationMessage,
	}
	return fabricValidationCode, event, nil
}

func MapValidationCode(code int32) (int, string) {
	return convertValidationCode(code), pb.TxValidationCode_name[code]
}

// GetChaincodeEvents reads the chaincode events and notifies the listeners registered to the specific chaincode.
func (c *Committer) GetChaincodeEvents(env *common.Envelope, blockNum driver2.BlockNum) error {
	chaincodeEvent, err := readChaincodeEvent(env, blockNum)
	if err != nil {
		return errors.Wrapf(err, "error reading chaincode event")
	}
	if chaincodeEvent != nil {
		logger.Debugf("Chaincode Event Received: ", chaincodeEvent)
		c.notifyChaincodeListeners(chaincodeEvent)
	}
	return nil
}

// CommitEndorserTransaction commits the transaction to the vault.
// It returns true, if the transaction was already processed, false otherwise.
func (c *Committer) CommitEndorserTransaction(ctx context.Context, txID string, blockNum driver2.BlockNum, indexInBlock uint64, env *common.Envelope, event *FinalityEvent) (bool, error) {
	logger.DebugfContext(ctx, "transaction [%s] in block [%d] is valid for fabric, commit!", txID, blockNum)

	event.Block = blockNum
	event.IndexInBlock = indexInBlock

	vc, _, err := c.Status(ctx, txID)
	if err != nil {
		return false, errors.Wrapf(err, "failed getting tx's status [%s]", txID)
	}

	switch vc {
	case driver.Valid:
		logger.Debugf("transaction [%s] in block [%d] is already marked as valid, skipping", txID, blockNum)
		// Nothing to commit
		return true, nil
	case driver.Invalid:
		logger.Debugf("transaction [%s] in block [%d] is marked as invalid, skipping", txID, blockNum)
		// Nothing to commit
		return true, nil
	}

	if err := c.CommitTX(ctx, event.TxID, event.Block, event.IndexInBlock, env); err != nil {
		return false, errors.Wrapf(err, "failed committing transaction [%s]", txID)
	}
	return false, nil
}

// DiscardEndorserTransaction discards the transaction from the vault
func (c *Committer) DiscardEndorserTransaction(ctx context.Context, txID string, blockNum driver2.BlockNum, envRaw []byte, event *FinalityEvent) error {
	logger.DebugfContext(ctx, "transaction [%s] in block [%d] is not valid for fabric [%s], discard!", txID, blockNum, event.ValidationCode)

	vc, _, err := c.Status(ctx, txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		// TODO: this might be due the fact that there are transactions with the same tx-id, the first is valid, the others are all invalid
		logger.Warnf("transaction [%s] in block [%d] is marked as valid but for fabric is invalid", txID, blockNum)
		return nil
	case driver.Invalid:
		logger.Debugf("transaction [%s] in block [%d] is marked as invalid, skipping", txID, blockNum)
		// Nothing to commit
		return nil
	case driver.Unknown:
		ok, err := c.filterUnknownEnvelope(ctx, txID, envRaw)
		if err != nil {
			return err
		}
		if ok {
			// so, we must remember that this transaction was discarded
			if err := c.EnvelopeService.StoreEnvelope(ctx, txID, envRaw); err != nil {
				return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
			}
			rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(ctx, txID)
			if err != nil {
				return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
			}
			rws.Done()
		}
	}

	event.Err = errors.Errorf("transaction [%s] status is not valid [%d], message [%s]", txID, event.ValidationCode, event.ValidationMessage)
	err = c.DiscardTx(ctx, event.TxID, event.ValidationMessage)
	if err != nil {
		logger.Errorf("failed discarding tx in state db with err [%s]", err)
	}
	return nil
}

func convertValidationCode(vc int32) driver.ValidationCode {
	switch pb.TxValidationCode(vc) {
	case pb.TxValidationCode_VALID:
		return driver.Valid
	default:
		return driver.Invalid
	}
}
