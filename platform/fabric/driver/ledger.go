/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type Block interface {
	DataAt(i int) []byte
}

type ProcessedTransaction interface {
	Results() []byte
	ValidationCode() int32
	IsValid() bool
}

// Ledger gives access to the remote ledger
type Ledger interface {
	// GetTransactionByID retrieves a transaction by id
	GetTransactionByID(txID string) (ProcessedTransaction, error)

	// GetBlockNumberByTxID returns the number of the block where the passed transaction appears
	GetBlockNumberByTxID(txID string) (uint64, error)

	// GetBlockByNumber fetches a block by number
	GetBlockByNumber(number uint64) (Block, error)
}
