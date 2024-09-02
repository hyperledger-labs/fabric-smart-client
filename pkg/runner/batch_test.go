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

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var counter uint32

func TestBatchRunner(t *testing.T) {
	atomic.StoreUint32(&counter, 0)
	runner, m, locksObtained := newBatchRunner()

	run(t, runner, 1000)
	assert.Len(t, m, 1000)
	assert.Equal(t, "val_10", m["key_10"])
	assert.Equal(t, 10, int(atomic.LoadUint32(locksObtained)))
}

func TestBatchRunnerFewRequests(t *testing.T) {
	atomic.StoreUint32(&counter, 0)
	runner, m, locksObtained := newBatchRunner()

	time.Sleep(100 * time.Millisecond)
	run(t, runner, 1)

	assert.Len(t, m, 1)
	assert.Equal(t, "val_1", m["key_1"])
	assert.Equal(t, 1, int(atomic.LoadUint32(locksObtained)))

	run(t, runner, 3)
	assert.Len(t, m, 4)
	assert.Equal(t, 2, int(atomic.LoadUint32(locksObtained)))
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

func run(t *testing.T, runner BatchRunner[int], times int) {
	var wg sync.WaitGroup
	wg.Add(times)
	for i := 0; i < times; i++ {
		v := int(atomic.AddUint32(&counter, 1))
		go func() {
			defer wg.Done()
			err := runner.Run(v)
			if v%10 == 0 {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		}()
	}
	wg.Wait()
}
