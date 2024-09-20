/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

func NewSerialRunner[V any](runner ExecuteFunc[V, error]) BatchRunner[V] {
	return &serialRunner[V]{executor: runner}
}

type serialRunner[V any] struct {
	executor ExecuteFunc[V, error]
}

func (r *serialRunner[V]) Run(val V) error {
	return r.executor([]V{val})[0]
}

func NewSerialExecutor[I any, O any](executor ExecuteFunc[I, Output[O]]) BatchExecutor[I, O] {
	return &serialExecutor[I, O]{executor: executor}
}

type serialExecutor[I any, O any] struct {
	executor ExecuteFunc[I, Output[O]]
}

func (r *serialExecutor[I, O]) Execute(input I) (O, error) {
	res := r.executor([]I{input})[0]
	return res.Val, res.Err
}
