/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"

var logger = logging.MustGetLogger()

// BatchExecutor executes operations on inputs and returns results with potential errors.
// Implementations may batch multiple Execute calls for efficiency.
type BatchExecutor[I any, O any] interface {
	// Execute processes the input and returns the result or an error.
	Execute(input I) (O, error)
}

// BatchRunner executes operations that may return errors.
// Implementations may batch multiple Run calls for efficiency.
type BatchRunner[V any] interface {
	// Run processes the value and returns an error if the operation fails.
	Run(v V) error
}

// Output represents the result of an execution with a value and optional error.
type Output[O any] struct {
	Val O
	Err error
}

// ExecuteFunc is a function that processes a batch of inputs and returns corresponding outputs.
type ExecuteFunc[I any, O any] func([]I) []O
