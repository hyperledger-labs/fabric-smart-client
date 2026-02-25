/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

// NewSerialRunner creates a BatchRunner that executes operations serially without batching.
// Each Run call is processed immediately by invoking the runner function with a single-element slice.
func NewSerialRunner[V any](runner ExecuteFunc[V, error]) BatchRunner[V] {
	return &serialRunner[V]{executor: runner}
}

type serialRunner[V any] struct {
	executor ExecuteFunc[V, error]
}

func (r *serialRunner[V]) Run(val V) error {
	return r.executor([]V{val})[0]
}

// NewSerialExecutor creates a BatchExecutor that executes operations serially without batching.
// Each Execute call is processed immediately by invoking the executor function with a single-element slice.
func NewSerialExecutor[I, O any](executor ExecuteFunc[I, Output[O]]) BatchExecutor[I, O] {
	return &serialExecutor[I, O]{executor: executor}
}

type serialExecutor[I any, O any] struct {
	executor ExecuteFunc[I, Output[O]]
}

func (r *serialExecutor[I, O]) Execute(input I) (O, error) {
	res := r.executor([]I{input})[0]
	return res.Val, res.Err
}
