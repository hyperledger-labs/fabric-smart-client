/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

type batcher[I any, O any] struct {
	idx      uint32
	inputs   []chan I
	outputs  []chan O
	locks    []sync.Mutex
	len      uint32
	executor ExecuteFunc[I, O]
	timeout  time.Duration
}

func newBatcher[I any, O any](executor func([]I) []O, capacity int, timeout time.Duration) *batcher[I, O] {
	inputs := make([]chan I, capacity)
	outputs := make([]chan O, capacity)
	locks := make([]sync.Mutex, capacity)
	for i := 0; i < capacity; i++ {
		inputs[i] = make(chan I)
		outputs[i] = make(chan O)
		locks[i] = sync.Mutex{}
	}

	e := &batcher[I, O]{
		inputs:   inputs,
		outputs:  outputs,
		locks:    locks,
		len:      uint32(capacity),
		executor: executor,
		timeout:  timeout,
	}
	go e.start()
	return e
}

func (r *batcher[I, O]) start() {
	var inputs []I
	ticker := time.NewTicker(r.timeout)
	firstIdx := uint32(0) // Points to the first element of a new cycle
	for {
		// If we fill a whole cycle, the elements will be from firstIdx % r.len to lastIdx % r.len
		var lastIdx uint32
		var lastElement I
		select {
		case lastElement = <-r.inputs[(firstIdx+r.len-1)%r.len]:
			lastIdx = firstIdx + r.len
			logger.Debugf("Execute because %d input channels are full", r.len)
		case <-ticker.C:
			lastIdx = atomic.LoadUint32(&r.idx)
			if lastIdx == firstIdx {
				logger.Debugf("No new elements. Skip execution...")
				continue
			}
			lastElement = <-r.inputs[(lastIdx-1)%r.len] // We read the lastElement here just to avoid code repetition
			logger.Debugf("Execute because timeout of %v passed", r.timeout)
		}
		logger.Debugf("Read batch range [%d,%d)", firstIdx, lastIdx)

		inputs = make([]I, lastIdx-firstIdx)
		for i := uint32(0); i < lastIdx-firstIdx-1; i++ {
			inputs[i] = <-r.inputs[(i+firstIdx)%r.len]
		}
		inputs[lastIdx-firstIdx-1] = lastElement
		ticker.Reset(r.timeout)

		logger.Debugf("Start execution for %d inputs", len(inputs))
		outs := r.executor(inputs)
		logger.Debugf("Execution finished with %d outputs", len(outs))
		if len(inputs) != len(outs) {
			panic(errors.Errorf("expected %d outputs, but got %d", len(inputs), len(outs)))
		}
		for i, err := range outs {
			r.outputs[(firstIdx+uint32(i))%r.len] <- err
		}
		logger.Debugf("Results distributed for range [%d,%d)", firstIdx, lastIdx)
		firstIdx = lastIdx
	}
}

func (r *batcher[I, O]) call(input I) O {
	idx := atomic.AddUint32(&r.idx, 1) - 1
	r.locks[idx%r.len].Lock()
	defer r.locks[idx%r.len].Unlock()
	r.inputs[idx%r.len] <- input
	logger.Debugf("Enqueued input [%d] and waiting for result", idx)
	defer logger.Debugf("Return result of output [%d]", idx)
	return <-r.outputs[idx%r.len]
}

type batchExecutor[I any, O any] struct {
	*batcher[I, Output[O]]
}

func NewBatchExecutor[I any, O any](executor ExecuteFunc[I, Output[O]], capacity int, timeout time.Duration) BatchExecutor[I, O] {
	return &batchExecutor[I, O]{batcher: newBatcher(executor, capacity, timeout)}
}

func (r *batchExecutor[I, O]) Execute(input I) (O, error) {
	o := r.call(input)
	return o.Val, o.Err
}

type batchRunner[V any] struct {
	*batcher[V, error]
}

func NewBatchRunner[V any](runner func([]V) []error, capacity int, timeout time.Duration) BatchRunner[V] {
	return &batchRunner[V]{batcher: newBatcher(runner, capacity, timeout)}
}

func (r *batchRunner[V]) Run(val V) error {
	return r.call(val)
}
