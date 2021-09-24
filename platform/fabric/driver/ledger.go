/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

// Block models a block of the ledger
type Block interface {
	// DataAt returns the data stored at the passed index
	DataAt(i int) []byte
	// ProcessedTransaction returns the ProcessedTransaction at passed index
	ProcessedTransaction(i int) (ProcessedTransaction, error)
}

// ProcessedTransaction models a transaction that has been processed by Fabric
type ProcessedTransaction interface {
	// TxID returns the transaction's id
	TxID() string
	// Results returns the rwset marshaled
	Results() []byte
	// ValidationCode of this transaction
	ValidationCode() int32
	// IsValid returns true if the transaction is valid, false otherwise
	IsValid() bool
	// Envelope returns the Fabric envelope
	Envelope() []byte
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
