/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
)

var (
	logger  = logging.MustGetLogger()
	retries = 5
)

// ledger is an in-memory implementation of the driver.Ledger interface.
// this component is here temporary until we have an implementation that can get this information from the committer directly.
type ledger struct {
	mu        sync.RWMutex
	statuses  map[string]committerpb.Status
	blockNums map[string]driver.BlockNum
}

// check that we implement the driver.Ledger.
var _ driver.Ledger = (*ledger)(nil)

func New() *ledger {
	l := &ledger{
		statuses:  make(map[string]committerpb.Status),
		blockNums: make(map[string]driver.BlockNum),
	}
	return l
}

func (c *ledger) OnBlock(_ context.Context, block *cb.Block) (bool, error) {
	logger.Debugf("Received block [blockNo=%d]", block.Header.Number)
	newStatuses := make(map[string]committerpb.Status, len(block.Data.Data))
	newBlockNums := make(map[string]driver.BlockNum, len(block.Data.Data))

	for i, tx := range block.Data.Data {
		_, _, chdr, err := fabricutils.UnmarshalTx(tx)
		if err != nil {
			return false, errors.Wrapf(err, "unmarshal transaction channel header")
		}

		statusCode := committerpb.Status(block.Metadata.Metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER][i])

		logger.Debugf("unmarshalled [blockNum=%d, pos=%d, txID=%s, status=%v]",
			block.Header.Number, i, chdr.TxId, statusCode)
		newStatuses[chdr.TxId] = statusCode
		newBlockNums[chdr.TxId] = block.Header.Number
	}

	c.mu.Lock()
	for txID, status := range newStatuses {
		c.statuses[txID] = status
	}
	for txID, blockNum := range newBlockNums {
		c.blockNums[txID] = blockNum
	}
	logger.Debugf("Current size of tx statuses: %d", len(c.statuses))
	c.mu.Unlock()

	return false, nil
}

func (*ledger) GetLedgerInfo() (*driver.LedgerInfo, error) {
	return nil, errors.New("not implemented")
}

func (c *ledger) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	// TODO: this will be replaced with a call to the sidecar
	logger.Debugf("Seek transaction status [%s]", txID)
	for range retries {
		c.mu.RLock()
		status, ok := c.statuses[txID]
		c.mu.RUnlock()
		if ok {
			logger.Debugf("Transaction [txID=%s] found with status [%d]", txID, int32(status))
			return &liteTx{txID: txID, validationCode: status}, nil
		}
		logger.Warnf("Transaction [txID=%s] not found. retrying...", txID)
		time.Sleep(1 * time.Second)
	}

	return nil, errors.Wrapf(finality.TxNotFound, "transaction [%s] not found", txID)
}

func (c *ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	// TODO: this will be replaced with a call to the sidecar
	logger.Debugf("Seek transaction blockNum [%s]", txID)
	for range retries {
		c.mu.RLock()
		blockNum, ok := c.blockNums[txID]
		c.mu.RUnlock()
		if ok {
			logger.Debugf("Transaction [txID=%s] found with blockNum [%v]", txID, blockNum)
			return blockNum, nil
		}
		logger.Warnf("Transaction [txID=%s] not found. retrying...", txID)
		time.Sleep(1 * time.Second)
	}
	return 0, errors.Errorf("transaction [txID=%s] not found", txID)
}

func (c *ledger) GetBlockByNumber(number uint64) (driver.Block, error) {
	// TODO: this will be replaced with a call to the sidecar
	panic("GetBlockByNumber >> implement me")
}

type liteTx struct {
	txID           string
	validationCode committerpb.Status
}

func (t *liteTx) TxID() string {
	return t.txID
}

func (t *liteTx) Results() []byte {
	panic("unimplemented Results()")
}

func (t *liteTx) ValidationCode() int32 {
	return int32(t.validationCode)
}

func (t *liteTx) IsValid() bool {
	return t.validationCode == committerpb.Status_COMMITTED
}

func (t *liteTx) Envelope() []byte {
	panic("unimplemented Envelope()")
}
