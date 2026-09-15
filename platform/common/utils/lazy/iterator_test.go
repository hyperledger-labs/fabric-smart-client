/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package lazy

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

func TestIteratorEmpty(t *testing.T) {
	t.Parallel()

	it := NewIterator[int]()

	v, err := it.Next()
	require.ErrorIs(t, err, io.EOF)
	require.Zero(t, v)
}

func TestIteratorSequential(t *testing.T) {
	t.Parallel()

	it := NewIterator(
		func() (int, error) { return 1, nil },
		func() (int, error) { return 2, nil },
		func() (int, error) { return 3, nil },
	)

	for _, want := range []int{1, 2, 3} {
		v, err := it.Next()
		require.NoError(t, err)
		require.Equal(t, want, v)
	}

	v, err := it.Next()
	require.ErrorIs(t, err, io.EOF)
	require.Zero(t, v)
}

func TestIteratorPropagatesErrorAndAdvances(t *testing.T) {
	t.Parallel()

	it := NewIterator(
		func() (int, error) { return 0, errors.New("boom") },
		func() (int, error) { return 2, nil },
	)

	v, err := it.Next()
	require.Error(t, err)
	require.Zero(t, v)

	// the failing func isn't retried: the next call moves on to the one after it
	v, err = it.Next()
	require.NoError(t, err)
	require.Equal(t, 2, v)

	_, err = it.Next()
	require.ErrorIs(t, err, io.EOF)
}

func TestIteratorClose(t *testing.T) {
	t.Parallel()

	it := NewIterator(
		func() (int, error) { return 1, nil },
	)

	it.Close()

	v, err := it.Next()
	require.ErrorIs(t, err, io.EOF)
	require.Zero(t, v)
}
