/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// StateReference models the index of an output
type StateReference struct {
	TxID  string // Transaction ID
	Index uint64 // Output’s index in the transaction
}

func (s StateReference) String() string {
	return fmt.Sprintf("[TxID:%s, Index:%d]", s.TxID, s.Index)
}

type Output struct {
	Reference *StateReference
	ID        string // Unique Identifier of this Output
	Raw       []byte // Raw representation of the state
	Birth     Script // Birth script descriptor
	Death     Script // Death script descriptor
}

func (o1 *Output) Equals(o2 *Output) bool {
	panic("implement me")
}

// Transaction provides a UTXO transaction abstraction
type Transaction interface {
	// ID returns the transaction's id
	ID() string

	// Inputs returns the inputs referenced by this transaction
	Inputs() []*StateReference

	// Outputs returns the outputs defined by this transaction
	Outputs() []*Output

	// NumInputs returns the number of inputs in this transaction
	NumInputs() int

	// NumOutputs returns the number of inputs in this transaction
	NumOutputs() int

	// GetInputAt retrieves the state referenced by the index-th input. In addition, it returns
	// the worldstate reference to that state and the associated birth and death scripts
	GetInputAt(index int, state interface{}) (*StateReference, *Scripts, error)

	// GetInputScriptsAt returns the birth and death scripts associated to worldstate
	// referenced by the index-th input
	GetInputScriptsAt(index int) (*Scripts, error)

	// GetOutputAt retrieves the state referenced by the index-th output. In addition, it returns
	// the associated birth and death scripts
	GetOutputAt(index int, state interface{}) (*Scripts, error)

	// GetOutputScriptsAt returns the birth and death scripts associated to transaction's output
	// referenced by the index-th input
	GetOutputScriptsAt(index int) (*Scripts, error)

	// HasBeenSignedBy returns nil if all the passed parties have endorsed this transaction
	HasBeenSignedBy(parties ...view.Identity) error

	// NumSignatures returns the number of signatures carried by this transaction
	NumSignatures() int
}
