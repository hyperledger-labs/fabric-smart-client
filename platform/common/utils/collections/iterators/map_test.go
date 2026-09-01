/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

// --- Map ------------------------------------------------------------------

func TestMap(t *testing.T) {
	t.Parallel()

	it := iterators.Map(iterators.Slice([]int{1, 2, 3}),
		func(v int) (string, error) { return strconv.Itoa(v), nil })

	got := make([]string, 0)
	for range 3 {
		v, err := it.Next()
		require.NoError(t, err)
		got = append(got, v)
	}
	require.Equal(t, []string{"1", "2", "3"}, got)
}

func TestMapTransformerError(t *testing.T) {
	t.Parallel()

	mapErr := errors.New("cannot map")
	it := iterators.Map(iterators.Slice([]int{1}),
		func(int) (string, error) { return "", mapErr })

	_, err := it.Next()
	require.ErrorIs(t, err, mapErr)
}

func TestMapUnderlyingError(t *testing.T) {
	t.Parallel()

	it := iterators.Map[int, int](newFailing([]int{1}, 0),
		func(v int) (int, error) { return v, nil })

	_, err := it.Next()
	require.ErrorIs(t, err, errAt)
}

// TestMapTransformsExhaustionSentinel documents that Map applies the transformer
// to the zero value produced at exhaustion, rather than short-circuiting on it.
// A transformer used with Map must therefore tolerate the zero value.
func TestMapTransformsExhaustionSentinel(t *testing.T) {
	t.Parallel()

	calls := 0
	it := iterators.Map(iterators.Slice([]int{1}), func(v int) (int, error) {
		calls++
		return v * 10, nil
	})

	first, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, 10, first)

	// Past the end: the underlying slice yields 0, which is still transformed.
	past, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, 0, past)
	require.Equal(t, 2, calls, "the transformer also runs for the exhaustion sentinel")
}

func TestMapClosePropagates(t *testing.T) {
	t.Parallel()

	rec := newCloseRecorder([]int{1})
	it := iterators.Map[int, int](rec, func(v int) (int, error) { return v, nil })
	it.Close()
	require.True(t, rec.closed)
}
