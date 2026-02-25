/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

func TestBatchRunner(t *testing.T) {
	t.Parallel()

	ctr := &atomic.Uint32{}
	runner, m, locksObtained := newBatchRunner()

	run(t, ctr, runner, 100)
	require.Len(t, m, 100)
	require.Equal(t, "val_10", m["key_10"])

	// as we spawn 100 concurrent requests, we might get either 1 or 2 locks
	require.LessOrEqual(t, int(atomic.LoadUint32(locksObtained)), 2, "should not exceed 2 batches for 100 items")
}

func TestBatchRunnerFewRequests(t *testing.T) {
	t.Parallel()

	ctr := &atomic.Uint32{}
	runner, m, locksObtained := newBatchRunner()

	run(t, ctr, runner, 1)

	require.Len(t, m, 1)
	require.Equal(t, "val_1", m["key_1"])
	require.Equal(t, 1, int(atomic.LoadUint32(locksObtained)))

	run(t, ctr, runner, 3)
	require.Len(t, m, 4)

	// as we spawn 3 concurrent, we might get either 2 or 3 locks
	l := int(atomic.LoadUint32(locksObtained))
	require.GreaterOrEqual(t, l, 2)
	require.LessOrEqual(t, l, 4)
}

func newBatchRunner() (BatchRunner[int], map[string]string, *uint32) {
	var locksObtained uint32
	m := make(map[string]string)
	var mu sync.RWMutex
	runner := NewBatchRunner(func(vs []int) []error {
		mu.Lock()
		atomic.AddUint32(&locksObtained, 1)
		defer mu.Unlock()
		errs := make([]error, len(vs))
		for i, v := range vs {
			m[fmt.Sprintf("key_%d", v)] = fmt.Sprintf("val_%d", v)
			if v%10 == 0 {
				errs[i] = errors.Errorf("error_%d", v)
			}
		}
		return errs
	}, 100, 10*time.Millisecond)
	return runner, m, &locksObtained
}

func run(t *testing.T, ctr *atomic.Uint32, runner BatchRunner[int], times int) {
	var wg sync.WaitGroup
	wg.Add(times)
	for i := 0; i < times; i++ {
		v := int(ctr.Add(1))
		go func() {
			defer wg.Done()
			err := runner.Run(v)
			if v%10 == 0 {
				assert.Error(t, err, "expected error for %d", v)
			} else {
				assert.NoError(t, err, "expected no error for %d", v)
			}
		}()
	}
	wg.Wait()
}

func TestBatchExecutor_Success(t *testing.T) {
	t.Parallel()

	executor := NewBatchExecutor(func(inputs []string) []Output[int] {
		outputs := make([]Output[int], len(inputs))
		for i, input := range inputs {
			outputs[i] = Output[int]{Val: len(input), Err: nil}
		}
		return outputs
	}, 10, 50*time.Millisecond)

	val, err := executor.Execute("hello")
	require.NoError(t, err)
	require.Equal(t, 5, val)
}

func TestBatchExecutor_Error(t *testing.T) {
	t.Parallel()

	executor := NewBatchExecutor(func(inputs []string) []Output[int] {
		outputs := make([]Output[int], len(inputs))
		for i, input := range inputs {
			if input == "error" {
				outputs[i] = Output[int]{Val: 0, Err: errors.New("test error")}
			} else {
				outputs[i] = Output[int]{Val: len(input), Err: nil}
			}
		}
		return outputs
	}, 10, 50*time.Millisecond)

	val, err := executor.Execute("error")
	require.Error(t, err)
	require.Equal(t, 0, val)
}

func TestBatchExecutor_Concurrent(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	processed := make(map[string]bool)

	executor := NewBatchExecutor(func(inputs []string) []Output[int] {
		mu.Lock()
		defer mu.Unlock()
		outputs := make([]Output[int], len(inputs))
		for i, input := range inputs {
			processed[input] = true
			outputs[i] = Output[int]{Val: len(input), Err: nil}
		}
		return outputs
	}, 5, 20*time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(10)
	for i := range 10 {
		input := fmt.Sprintf("input_%d", i)
		go func(s string) {
			defer wg.Done()
			val, err := executor.Execute(s)
			assert.NoError(t, err)
			assert.Equal(t, len(s), val)
		}(input)
	}
	wg.Wait()

	require.Len(t, processed, 10)
}

func TestBatchRunner_MultipleCycles(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	totalProcessed := 0

	runner := NewBatchRunner(func(vals []int) []error {
		mu.Lock()
		totalProcessed += len(vals)
		mu.Unlock()
		return make([]error, len(vals))
	}, 5, 20*time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(15)
	for i := range 15 {
		go func(val int) {
			defer wg.Done()
			err := runner.Run(val)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	require.Equal(t, 15, totalProcessed)
}

func TestBatchRunner_TimeoutWithNoRequests(t *testing.T) {
	t.Parallel()

	executionCount := atomic.Int32{}

	runner := NewBatchRunner(func(vals []int) []error {
		executionCount.Add(1)
		return make([]error, len(vals))
	}, 5, 10*time.Millisecond)

	// Wait for multiple timeout periods
	time.Sleep(50 * time.Millisecond)

	// Should not have executed
	require.Equal(t, int32(0), executionCount.Load())

	// Send one request
	err := runner.Run(1)
	require.NoError(t, err)

	// Should have executed once
	require.Equal(t, int32(1), executionCount.Load())
}
