/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type LedgerInfo = driver.LedgerInfo

// Block models a Fabric block
type Block struct {
	b driver.Block
}

// DataAt returns the data stored at the passed index
func (b *Block) DataAt(i int) []byte {
	return b.b.DataAt(i)
}

// ProcessedTransaction models a transaction that has been processed by Fabric
type ProcessedTransaction struct {
	pt driver.ProcessedTransaction
}

// TxID returns the transaction's id
func (pt *ProcessedTransaction) TxID() string {
	return pt.pt.TxID()
}

// Results returns the rwset marshaled
func (pt *ProcessedTransaction) Results() []byte {
	return pt.pt.Results()
}

// ValidationCode of this transaction
func (pt *ProcessedTransaction) ValidationCode() int32 {
	return pt.pt.ValidationCode()
}

// Ledger models the ledger stored at a remote Fabric peer
type Ledger struct {
	l driver.Ledger
}

// GetBlockNumberByTxID returns the number of the block where the passed transaction appears
func (l *Ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	return l.l.GetBlockNumberByTxID(txID)
}

// GetTransactionByID retrieves a transaction by id
func (l *Ledger) GetTransactionByID(txID string) (*ProcessedTransaction, error) {
	pt, err := l.l.GetTransactionByID(txID)
	if err != nil {
		return nil, err
	}
	return &ProcessedTransaction{pt: pt}, nil
}

// GetBlockByNumber fetches a block by number
func (l *Ledger) GetBlockByNumber(number uint64) (*Block, error) {
	b, err := l.l.GetBlockByNumber(number)
	if err != nil {
		return nil, err
	}
	return &Block{b: b}, nil
}

func (l *Ledger) GetLedgerInfo() (*LedgerInfo, error) {
	return l.l.GetLedgerInfo()
}
