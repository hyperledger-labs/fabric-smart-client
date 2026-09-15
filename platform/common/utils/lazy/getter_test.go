/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetterBasic(t *testing.T) {
	t.Parallel()

	count := 0
	g := NewGetter(func() (int, error) {
		count++
		return 42, nil
	})

	v, err := g.Get()
	require.NoError(t, err)
	require.Equal(t, 42, v)

	v, err = g.Get()
	require.NoError(t, err)
	require.Equal(t, 42, v)
	require.Equal(t, 1, count)
}

func TestGetterErrors(t *testing.T) {
	t.Parallel()

	g := NewGetter(func() (int, error) {
		return 0, errors.New("provider error")
	})

	v, err := g.Get()
	require.Error(t, err)
	require.Equal(t, "provider error", err.Error())
	require.Equal(t, 0, v)
}

func TestGetterConcurrent(t *testing.T) {
	t.Parallel()

	var count int
	var mu sync.Mutex
	g := NewGetter(func() (int, error) {
		mu.Lock()
		count++
		mu.Unlock()
		return 42, nil
	})

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			v, err := g.Get()
			require.NoError(t, err)
			require.Equal(t, 42, v)
		})
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, count)
}
