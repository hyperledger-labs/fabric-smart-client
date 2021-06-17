/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type Block struct {
	b driver.Block
}

func (b *Block) DataAt(i int) []byte {
	return b.DataAt(i)
}

type ProcessedTransaction struct {
	pt driver.ProcessedTransaction
}

func (pt *ProcessedTransaction) Results() []byte {
	return pt.pt.Results()
}

type Ledger struct {
	ch *Channel
}

func (l *Ledger) GetBlockNumberByTxID(txID string) (uint64, error) {
	return l.ch.ch.GetBlockNumberByTxID(txID)
}

func (l *Ledger) GetTransactionByID(txID string) (*ProcessedTransaction, error) {
	pt, err := l.ch.ch.GetTransactionByID(txID)
	if err != nil {
		return nil, err
	}
	return &ProcessedTransaction{pt: pt}, nil
}

func (l *Ledger) GetBlockByNumber(number uint64) (*Block, error) {
	b, err := l.ch.ch.GetBlockByNumber(number)
	if err != nil {
		return nil, err
	}
	return &Block{b: b}, nil
}
