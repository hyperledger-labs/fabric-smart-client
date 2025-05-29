/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type ValidationCode = int

const (
	_       ValidationCode = iota
	Valid                  // Transaction is valid and committed
	Invalid                // Transaction is invalid and has been discarded
	Busy                   // Transaction does not yet have a validity state
	Unknown                // Transaction is unknown
)

var ValidationCodeProvider = driver.NewValidationCodeProvider(map[ValidationCode]driver.TxStatusCode{
	Valid:   driver.Valid,
	Invalid: driver.Invalid,
	Busy:    driver.Busy,
	Unknown: driver.Unknown,
})

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
	Exists(ctx context.Context, txid string) bool
	StoreTransient(ctx context.Context, txid string, transientMap TransientMap) error
	LoadTransient(ctx context.Context, txid string) (TransientMap, error)
}

type EnvelopeService interface {
	Exists(ctx context.Context, txid string) bool
	StoreEnvelope(ctx context.Context, txid string, env interface{}) error
	LoadEnvelope(ctx context.Context, txid string) ([]byte, error)
}

type TransactionService interface {
	Exists(ctx context.Context, txid string) bool
	StoreTransaction(ctx context.Context, txid string, raw []byte) error
	LoadTransaction(ctx context.Context, txid string) ([]byte, error)
}
