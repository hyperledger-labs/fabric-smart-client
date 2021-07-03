/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

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

// Results return the rwset marshaled
func (pt *ProcessedTransaction) Results() []byte {
	return pt.pt.Results()
}

// Ledger models the ledger stored at a remote Fabric peer
type Ledger struct {
	ch *Channel
}

// GetBlockNumberByTxID returns the number of the block where the passed transaction appears
func (l *Ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	return l.ch.ch.GetBlockNumberByTxID(txID)
}

// GetTransactionByID retrieves a transaction by id
func (l *Ledger) GetTransactionByID(txID string) (*ProcessedTransaction, error) {
	pt, err := l.ch.ch.GetTransactionByID(txID)
	if err != nil {
		return nil, err
	}
	return &ProcessedTransaction{pt: pt}, nil
}

// GetBlockByNumber fetches a block by number
func (l *Ledger) GetBlockByNumber(number uint64) (*Block, error) {
	b, err := l.ch.ch.GetBlockByNumber(number)
	if err != nil {
		return nil, err
	}
	return &Block{b: b}, nil
}
