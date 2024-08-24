/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

type (
	// ParallelExecutable is a marker type for processes that can take place in parallel
	ParallelExecutable[V any] []V
	// SerialExecutable is a marker type for processes that must take place serially
	SerialExecutable[V any] []V
)

// DependencyResolver analyzes the dependencies between the transactions of a block
// and indicates which commits can take place in parallel and which have to take place serially
type DependencyResolver interface {
	// Resolve returns a two-dimensional array that indicates how commits can be parallelized.
	// The transactions in each SerialExecutable slice have to follow the order indicated.
	// Each slice of the ParallelExecutable slice can be committed independently without waiting on the others.
	// Hence, for each element of the ParallelExecutable slice, a new goroutine can be launched.
	Resolve([]CommitTx) ParallelExecutable[SerialExecutable[CommitTx]]
}

// serialDependencyResolver returns a ParallelExecutable slice with only one element.
// This means that we can only launch one goroutine that commits each transaction one after the other (safe approach)
type serialDependencyResolver struct{}

func NewSerialDependencyResolver() *serialDependencyResolver {
	return &serialDependencyResolver{}
}

func (r *serialDependencyResolver) Resolve(txs []CommitTx) ParallelExecutable[SerialExecutable[CommitTx]] {
	return ParallelExecutable[SerialExecutable[CommitTx]]{txs}
}

// parallelDependencyResolver returns a ParallelExecutable slice with len(txs) elements.
// This means that all transactions can be committed independently (for UTXO transactions)
type parallelDependencyResolver struct{}

func NewParallelDependencyResolver() *parallelDependencyResolver {
	return &parallelDependencyResolver{}
}

func (r *parallelDependencyResolver) Resolve(txs []CommitTx) ParallelExecutable[SerialExecutable[CommitTx]] {
	s := make(ParallelExecutable[SerialExecutable[CommitTx]], len(txs))
	for i, tx := range txs {
		s[i] = SerialExecutable[CommitTx]{tx}
	}
	return s
}
