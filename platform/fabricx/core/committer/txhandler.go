/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
)

const statusIdx = int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER)

var logger = logging.MustGetLogger()

func RegisterTransactionHandler(com *committer.Committer) {
	h := NewHandler(com)
	com.Handlers[cb.HeaderType_MESSAGE] = h.HandleFabricxTransaction
}

func NewHandler(com *committer.Committer) *handler {
	return &handler{committer: com}
}

type handler struct {
	committer *committer.Committer
}

func (h *handler) HandleFabricxTransaction(ctx context.Context, blkMetadata *cb.BlockMetadata, tx committer.CommitTx) (*committer.FinalityEvent, error) {
	if len(blkMetadata.Metadata) < statusIdx {
		return nil, errors.New("block metadata lacks transaction filter")
	}

	statusCode := committerpb.Status(blkMetadata.Metadata[statusIdx][tx.TxNum])
	event := &committer.FinalityEvent{
		Ctx:               ctx,
		TxID:              tx.TxID,
		ValidationCode:    convertValidationCode(statusCode),
		ValidationMessage: statusCode.String(),
	}
	logger.Debugf("handle transaction [txID=%s] [status=%s]", tx.TxID, statusCode.String())

	switch statusCode {
	case committerpb.Status_COMMITTED:
		processed, err := h.committer.CommitEndorserTransaction(ctx, event.TxID, tx.BlkNum, tx.TxNum, tx.Envelope, event)
		if err != nil {
			if errors.HasCause(err, committer.ErrDiscardTX) {
				// in this case, we will discard the transaction
				event.ValidationCode = driver.Invalid
				event.ValidationMessage = err.Error()

				// escaping the switch and discard
				break
			}
			return nil, errors.Wrapf(err, "committing endorser transaction [txID=%s]", event.TxID)
		}
		if !processed {
			logger.Debugf("TODO: Should we try to get chaincode events?")
			// if err := h.committer.GetChaincodeEvents(tx.Envelope, tx.BlkNum); err != nil {
			//	return nil, fmt.Errorf("failed to publish chaincode events [%s]: %w", event.TxID, err)
			//}
		}
		return event, nil
	}

	logger.Warnf("discarding transaction [txID=%s] [reason=%v]", tx.TxID, statusCode.String())
	if err := h.committer.DiscardEndorserTransaction(ctx, event.TxID, tx.BlkNum, tx.Raw, event); err != nil {
		return nil, errors.Wrapf(err, "discarding endorser transaction [txID=%s]", event.TxID)
	}

	return event, nil
}

func convertValidationCode(status committerpb.Status) driver.ValidationCode {
	switch status {
	case committerpb.Status_COMMITTED:
		return driver.Valid
	default:
		return driver.Invalid
	}
}
