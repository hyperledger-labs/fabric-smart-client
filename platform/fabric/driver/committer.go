/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
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

var (
	// ValidationCodeMessage maps ValidationCode to string
	ValidationCodeMessage = map[ValidationCode]string{
		Valid:   "Valid",
		Invalid: "Invalid",
		Busy:    "Busy",
		Unknown: "Unknown",
	}
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
type FinalityListener = committer.FinalityListener[ValidationCode]

type TransactionFilter = driver.TransactionFilter

// Committer models the committer service
type Committer interface {
	Start(context context.Context) error

	// ProcessNamespace registers namespaces that will be committed even if the rwset is not known
	ProcessNamespace(nss ...string) error

	// AddTransactionFilter adds a new transaction filter to this commit pipeline.
	// The transaction filter is used to check if an unknown transaction needs to be processed anyway
	AddTransactionFilter(tf TransactionFilter) error

	// Status returns a validation code this committer bind to the passed transaction id, plus
	// a list of dependant transaction ids if they exist.
	Status(txID string) (ValidationCode, string, error)

	// AddFinalityListener registers a listener for transaction status changes for the passed transaction id.
	// If the transaction id is empty, the listener will be called for all transactions.
	AddFinalityListener(txID string, listener FinalityListener) error

	// RemoveFinalityListener unregisters the passed listener.
	RemoveFinalityListener(txID string, listener FinalityListener) error
}
