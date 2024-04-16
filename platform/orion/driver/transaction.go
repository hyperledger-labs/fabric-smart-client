/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type ValidationCode = int

const (
	_       ValidationCode = iota
	Valid                  // Transaction is valid and committed
	Invalid                // Transaction is invalid and has been discarded
	Busy                   // Transaction does not yet have a validity state
	Unknown                // Transaction is unknown
)

type Envelope interface {
	TxID() string
	Nonce() []byte
	Creator() []byte
	Results() []byte
	Bytes() ([]byte, error)
	FromBytes(raw []byte) error
	String() string
}

type TxID struct {
	Nonce   []byte
	Creator []byte
}

type TransactionManager interface {
	ComputeTxID(id *TxID) string
	NewEnvelope() Envelope
	CommitEnvelope(session Session, envelope Envelope) error
}

type TransientMap map[string][]byte

type MetadataService interface {
	Exists(txid string) bool
	StoreTransient(txid string, transientMap TransientMap) error
	LoadTransient(txid string) (TransientMap, error)
}

type EnvelopeService interface {
	Exists(txid string) bool
	StoreEnvelope(txid string, env interface{}) error
	LoadEnvelope(txid string) ([]byte, error)
}

type TransactionService interface {
	Exists(txid string) bool
	StoreTransaction(txid string, raw []byte) error
	LoadTransaction(txid string) ([]byte, error)
}
