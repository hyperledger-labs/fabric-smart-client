/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type ValidationCode int

const (
	_               ValidationCode = iota
	Valid                          // Transaction is valid and committed
	Invalid                        // Transaction is invalid and has been discarded
	Busy                           // Transaction does not yet have a validity state
	Unknown                        // Transaction is unknown
	HasDependencies                // Transaction is unknown but has known dependencies
)

type Envelope interface {
	TxID() string
	Nonce() []byte
	Creator() []byte
	Results() []byte
	Bytes() ([]byte, error)
	FromBytes(raw []byte) error
}

type TxID struct {
	Nonce   []byte
	Creator []byte
}

type TransactionManager interface {
	ComputeTxID(id *TxID) string
	NewEnvelope() Envelope
}

type TransientMap map[string][]byte

type MetadataService interface {
	Exists(txid string) bool
	StoreTransient(txid string, transientMap TransientMap) error
	LoadTransient(txid string) (TransientMap, error)
}

type EnvelopeService interface {
	Exists(txid string) bool
	StoreEnvelope(txid string, env []byte) error
	LoadEnvelope(txid string) ([]byte, error)
}
