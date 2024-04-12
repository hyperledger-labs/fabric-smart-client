/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type TxValidationStatus struct {
	TxID           string
	ValidationCode ValidationCode
	Message        string
}

// Vault models a key value store that can be updated by committing rwsets
type Vault interface {
	// NewQueryExecutor gives handle to a query executor.
	// A client can obtain more than one 'QueryExecutor's for parallel execution.
	// Any synchronization should be performed at the implementation level if required
	NewQueryExecutor() (QueryExecutor, error)

	// NewRWSet returns a RWSet for this ledger.
	// A client may obtain more than one such simulator; they are made unique
	// by way of the supplied txid
	NewRWSet(txid string) (RWSet, error)

	// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
	// from the passed bytes.
	// A client may obtain more than one such simulator; they are made unique
	// by way of the supplied txid
	GetRWSet(txid string, rwset []byte) (RWSet, error)

	// GetEphemeralRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
	// from the passed bytes.
	// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
	GetEphemeralRWSet(rwset []byte, namespaces ...string) (RWSet, error)

	CommitTX(id string, block uint64, index int) error

	Status(id string) (ValidationCode, string, error)

	Statuses(ids ...string) ([]TxValidationStatus, error)

	DiscardTx(id string, message string) error

	RWSExists(id string) bool

	Match(id string, results []byte) error
	Close() error
}
