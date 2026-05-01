/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dummyCloser struct {
	mu sync.Mutex
	v  int
}

func (d *dummyCloser) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.v = 0
	return nil
}

func TestLazyHolderRaceCondition(t *testing.T) {
	t.Parallel()
	// Provider function for the lazyHolder.
	provider := func() (*dummyCloser, error) {
		time.Sleep(10 * time.Millisecond) // Simulate some delay
		return &dummyCloser{v: 42}, nil
	}

	// Test for race conditions
	holder := NewCloserHolder(provider)

	var wg sync.WaitGroup

	// Concurrently call Get.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := holder.Get()
			if err != nil {
				t.Errorf("Get() error: %v", err)
			}
			if v == nil {
				t.Errorf("Get() returned nil value")
			}
		}()
	}

	// Concurrently call Reset.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := holder.Reset()
			if err != nil && !errors.Is(err, io.EOF) {
				t.Errorf("Reset() error: %v", err)
			}
		}()
	}

	wg.Wait()
}

func TestHolderBasic(t *testing.T) {
	t.Parallel()

	count := 0
	provider := func() (int, error) {
		count++
		return 42, nil
	}

	h := NewHolder(provider, func(v int) error { return nil })

	// Test Get
	v, err := h.Get()
	require.NoError(t, err)
	require.Equal(t, 42, v)
	require.Equal(t, 1, count)

	// Test Get again (cached)
	v, err = h.Get()
	require.NoError(t, err)
	require.Equal(t, 42, v)
	require.Equal(t, 1, count)
}

func TestHolderErrors(t *testing.T) {
	t.Parallel()

	provider := func() (int, error) {
		return 0, errors.New("provider error")
	}

	h := NewHolder(provider, func(v int) error { return nil })

	v, err := h.Get()
	require.Error(t, err)
	require.Equal(t, "provider error", err.Error())
	require.Equal(t, 0, v)
}

func TestHolderReset(t *testing.T) {
	t.Parallel()

	count := 0
	provider := func() (int, error) {
		count++
		return count, nil
	}

	closed := false
	closer := func(v int) error {
		closed = true
		if v == 2 {
			return errors.New("closer error")
		}
		return nil
	}

	h := NewHolder(provider, closer)

	// Get first value
	v, err := h.Get()
	require.NoError(t, err)
	require.Equal(t, 1, v)

	// Reset
	err = h.Reset()
	require.NoError(t, err)
	require.True(t, closed)

	// Get second value
	v, err = h.Get()
	require.NoError(t, err)
	require.Equal(t, 2, v)

	// Reset with error
	err = h.Reset()
	require.Error(t, err)
	require.Equal(t, "closer error", err.Error())
}

func TestResetEmpty(t *testing.T) {
	t.Parallel()
	h := NewHolder(func() (int, error) { return 42, nil }, func(int) error { return nil })
	err := h.Reset()
	require.NoError(t, err)
}
