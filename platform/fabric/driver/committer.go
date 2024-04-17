/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger/fabric-protos-go/common"
)

// ValidationCode of transaction
type ValidationCode = int

const (
	_       ValidationCode = iota
	Valid                  // Transaction is valid and committed
	Invalid                // Transaction is invalid and has been discarded
	Busy                   // Transaction does not yet have a validity state
	Unknown                // Transaction is unknown
)

type ValidationCodeProvider struct{}

func (p *ValidationCodeProvider) ToInt32(code ValidationCode) int32 { return int32(code) }
func (p *ValidationCodeProvider) FromInt32(code int32) ValidationCode {
	return ValidationCode(code)
}
func (p *ValidationCodeProvider) Unknown() ValidationCode { return Unknown }
func (p *ValidationCodeProvider) Valid() ValidationCode   { return Valid }
func (p *ValidationCodeProvider) Invalid() ValidationCode { return Invalid }
func (p *ValidationCodeProvider) Busy() ValidationCode    { return Busy }

// TransactionStatusChanged is sent when the status of a transaction changes
type TransactionStatusChanged struct {
	ThisTopic         string
	TxID              string
	VC                ValidationCode
	ValidationMessage string
}

// Topic returns the topic for the transaction status change
func (t *TransactionStatusChanged) Topic() string {
	return t.ThisTopic
}

// Message returns the message for the transaction status change
func (t *TransactionStatusChanged) Message() interface{} {
	return t
}

// FinalityListener is the interface that must be implemented to receive transaction status notifications
type FinalityListener interface {
	// OnStatus is called when the status of a transaction changes, or it is valid or invalid
	OnStatus(txID string, status ValidationCode, statusMessage string)
}

type StatusReporter interface {
	Status(txID string) (ValidationCode, string, []string, error)
}

// Committer models the committer service
type Committer interface {
	Start(context context.Context) error

	// ProcessNamespace registers namespaces that will be committed even if the rwset is not known
	ProcessNamespace(nss ...string) error

	// AddStatusReporter adds an external status reporter that can be used to understand
	// if a given transaction is known.
	AddStatusReporter(sr StatusReporter) error

	// Status returns a validation code this committer bind to the passed transaction id, plus
	// a list of dependant transaction ids if they exist.
	Status(txID string) (ValidationCode, string, error)

	// DiscardTx discards the transaction with the passed id and all its dependencies, if they exists.
	DiscardTx(txID string, message string) error

	// CommitTX commits the transaction with the passed id and all its dependencies, if they exists.
	// Depending on tx's status, CommitTX does the following:
	// Tx is Unknown, CommitTx does nothing and returns no error.
	// Tx is Valid, CommitTx does nothing and returns an error.
	// Tx is Invalid, CommitTx does nothing and returns an error.
	// Tx is Busy, if Tx is a multi-shard private transaction then CommitTx proceeds with the multi-shard private transaction commit protocol,
	// otherwise, CommitTx commits the transaction.
	CommitTX(txid string, block uint64, indexInBloc int, envelope *common.Envelope) error

	// CommitConfig commits the passed configuration envelope.
	CommitConfig(blockNumber uint64, raw []byte, envelope *common.Envelope) error

	// AddFinalityListener registers a listener for transaction status for the passed transaction id.
	// If the status is already valid or invalid, the listener is called immediately.
	// When the listener is invoked, then it is also removed.
	// If the transaction id is empty, the listener will be called on status changes of any transaction.
	// In this case, the listener is not removed
	AddFinalityListener(txID string, listener FinalityListener) error

	// RemoveFinalityListener unregisters the passed listener.
	RemoveFinalityListener(txID string, listener FinalityListener) error
}
