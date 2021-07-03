/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

// ValidationCode of transaction
type ValidationCode int

const (
	_               ValidationCode = iota
	Valid                          // Transaction is valid and committed
	Invalid                        // Transaction is invalid and has been discarded
	Busy                           // Transaction does not yet have a validity state
	Unknown                        // Transaction is unknown
	HasDependencies                // Transaction is unknown but has known dependencies
)

type Committer interface {
	// ProcessNamespace registers namespaces that will committed even if the rwset is not known
	ProcessNamespace(nss ...string) error

	// AddProcessor(ns string, processor Processor) error

	// Status returns a validation code this committer bind to the passed transaction id, plus
	// a list of dependant transaction ids if they exist.
	Status(txid string) (ValidationCode, []string, error)

	// DiscardTx discards the transaction with the passed id and all its dependencies, if they exists.
	DiscardTx(txid string) error

	// CommitTX commits the transaction with the passed id and all its dependencies, if they exists.
	// Depending on tx's status, CommitTX does the following:
	// Tx is Unknown, CommitTx does nothing and returns no error.
	// Tx is HasDependencies, CommitTx proceeds with the multi-shard private transaction commit protocol.
	// Tx is Valid, CommitTx does nothing and returns an error.
	// Tx is Invalid, CommitTx does nothing and returns an error.
	// Tx is Busy, if Tx is a multi-shard private transaction then CommitTx proceeds with the multi-shard private transaction commit protocol,
	// otherwise, CommitTx commits the transaction.
	CommitTX(txid string, block uint64, indexInBloc int, envelope []byte) error

	// CommitConfig commits the passed configuration envelope.
	CommitConfig(blockNumber uint64, envelope []byte) error
}
