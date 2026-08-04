/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type batcher[I any, O any] struct {
	ctx      context.Context
	idx      uint32
	inputs   []chan I
	outputs  []chan O
	locks    []sync.Mutex
	len      uint32
	executor ExecuteFunc[I, O]
	timeout  time.Duration
}

func newBatcher[I, O any](ctx context.Context, executor func([]I) []O, capacity int, timeout time.Duration) *batcher[I, O] {
	inputs := make([]chan I, capacity)
	outputs := make([]chan O, capacity)
	locks := make([]sync.Mutex, capacity)
	for i := range capacity {
		inputs[i] = make(chan I)
		outputs[i] = make(chan O)
		locks[i] = sync.Mutex{}
	}

	e := &batcher[I, O]{
		ctx:      ctx,
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
	defer ticker.Stop()
	firstIdx := uint32(0) // Points to the first element of a new cycle
	for {
		// If we fill a whole cycle, the elements will be from firstIdx % r.len to lastIdx % r.len
		var lastIdx uint32
		var lastElement I
		select {
		case <-r.ctx.Done():
			return
		case lastElement = <-r.inputs[(firstIdx+r.len-1)%r.len]:
			lastIdx = firstIdx + r.len
			logger.Debugf("Execute because %d input channels are full", r.len)
		case <-ticker.C:
			lastIdx = atomic.LoadUint32(&r.idx)
			if lastIdx == firstIdx {
				logger.Debugf("No new elements. Skip execution...")
				continue
			}
			// We read the lastElement here just to avoid code repetition
			select {
			case lastElement = <-r.inputs[(lastIdx-1)%r.len]:
			case <-r.ctx.Done():
				return
			}
			logger.Debugf("Execute because timeout of %v passed", r.timeout)
		}
		logger.Debugf("Read batch range [%d,%d)", firstIdx, lastIdx)

		inputs = make([]I, lastIdx-firstIdx)
		for i := uint32(0); i < lastIdx-firstIdx-1; i++ {
			select {
			case inputs[i] = <-r.inputs[(i+firstIdx)%r.len]:
			case <-r.ctx.Done():
				return
			}
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
			select {
			case r.outputs[(firstIdx+uint32(i))%r.len] <- err:
			case <-r.ctx.Done():
				return
			}
		}
		logger.Debugf("Results distributed for range [%d,%d)", firstIdx, lastIdx)
		firstIdx = lastIdx
	}
}

func (r *batcher[I, O]) call(input I) O {
	var zero O
	idx := atomic.AddUint32(&r.idx, 1) - 1
	r.locks[idx%r.len].Lock()
	defer r.locks[idx%r.len].Unlock()
	select {
	case r.inputs[idx%r.len] <- input:
	case <-r.ctx.Done():
		return zero
	}
	logger.Debugf("Enqueued input [%d] and waiting for result", idx)
	select {
	case out := <-r.outputs[idx%r.len]:
		logger.Debugf("Return result of output [%d]", idx)
		return out
	case <-r.ctx.Done():
		logger.Debugf("Context cancelled while waiting for result of output [%d]", idx)
		return zero
	}
}

type batchExecutor[I any, O any] struct {
	*batcher[I, Output[O]]
}

// NewBatchExecutor creates a BatchExecutor that batches multiple Execute calls for efficiency.
// Batching occurs when capacity is reached or timeout expires.
// The executor function receives a batch of inputs and must return corresponding outputs.
// If ctx is cancelled while a call is in flight, that call's Execute returns the zero
// value of O and a nil error, which must not be mistaken for a successful result.
func NewBatchExecutor[I, O any](
	ctx context.Context,
	executor ExecuteFunc[I, Output[O]],
	capacity int,
	timeout time.Duration,
) BatchExecutor[I, O] {
	return &batchExecutor[I, O]{batcher: newBatcher(ctx, executor, capacity, timeout)}
}

func (r *batchExecutor[I, O]) Execute(input I) (O, error) {
	o := r.call(input)
	return o.Val, o.Err
}

type batchRunner[V any] struct {
	*batcher[V, error]
}

// NewBatchRunner creates a BatchRunner that batches multiple Run calls for efficiency.
// Batching occurs when capacity is reached or timeout expires.
// The runner function receives a batch of values and must return corresponding errors.
// If ctx is cancelled while a call is in flight, that call's Run returns a nil error,
// which must not be mistaken for a successful run.
func NewBatchRunner[V any](ctx context.Context, runner func([]V) []error, capacity int, timeout time.Duration) BatchRunner[V] {
	return &batchRunner[V]{batcher: newBatcher(ctx, runner, capacity, timeout)}
}

func (r *batchRunner[V]) Run(val V) error {
	return r.call(val)
}
