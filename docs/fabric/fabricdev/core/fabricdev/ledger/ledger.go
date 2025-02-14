/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type Ledger struct {
}

func New() *Ledger {
	return &Ledger{}
}

func (c *Ledger) GetLedgerInfo() (*driver.LedgerInfo, error) {
	panic("implement me")
}

func (c *Ledger) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	panic("implement me")
}

func (c *Ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	panic("implement me")
}

func (c *Ledger) GetBlockByNumber(number uint64) (driver.Block, error) {
	panic("implement me")
}
