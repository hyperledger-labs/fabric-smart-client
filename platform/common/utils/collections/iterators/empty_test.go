/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package iterators_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
)

// --- Empty ----------------------------------------------------------------

func TestEmpty(t *testing.T) {
	t.Parallel()

	it := iterators.Empty[*int]()

	v, err := it.Next()
	require.NoError(t, err)
	require.Nil(t, v)

	it.Close() // must not panic

	got, err := iterators.ReadAllValues(iterators.Empty[*int]())
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestEmptyValueTypeYieldsZero(t *testing.T) {
	t.Parallel()

	v, err := iterators.Empty[int]().Next()
	require.NoError(t, err)
	require.Zero(t, v)
}

func TestEmptyCloseIsSafe(t *testing.T) {
	t.Parallel()

	it := iterators.Empty[int]()
	require.NotPanics(t, it.Close)
	require.NotPanics(t, it.Close, "Close is idempotent")
}
