/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// KeyDataStore is the get/exists/put surface the envelope, endorsement-transaction
// and metadata stores share under their own method names.
type KeyDataStore struct {
	Get    func(context.Context, string) ([]byte, error)
	Exists func(context.Context, string) (bool, error)
	Put    func(context.Context, string, []byte) error
}

// TestKeyDataStore checks s against a real database. A put value is read back
// and an unknown key is absent. A second put under the same key with a
// different value keeps the first value and, when strict, returns an error
// wrapping driver.ErrConflict, or nil otherwise. When strict, putting the
// stored value again returns nil, and of two concurrent puts of different
// values under one key exactly one succeeds.
func TestKeyDataStore(t *testing.T, s KeyDataStore, strict bool) {
	t.Helper()
	ctx := t.Context()
	// Not valid UTF-8 and containing a NUL, so a column that is not BYTEA rejects it.
	first := []byte{0x00, 0xff, 'f'}

	require.NoError(t, s.Put(ctx, "tx1", first))
	err := s.Put(ctx, "tx1", []byte("second"))
	if strict {
		require.ErrorIs(t, err, driver.ErrConflict)
		require.NoError(t, s.Put(ctx, "tx1", first), "putting the stored value again is not an error")
	} else {
		require.NoError(t, err, "putting an existing key is not an error")
	}

	got, err := s.Get(ctx, "tx1")
	require.NoError(t, err)
	assert.Equal(t, first, got, "the first put wins")

	exists, err := s.Exists(ctx, "tx1")
	require.NoError(t, err)
	assert.True(t, exists)

	got, err = s.Get(ctx, "unknown")
	require.NoError(t, err)
	assert.Empty(t, got, "an unknown key returns nothing")

	exists, err = s.Exists(ctx, "unknown")
	require.NoError(t, err)
	assert.False(t, exists)

	if !strict {
		return
	}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { errs[i] = s.Put(ctx, "race", []byte{byte(i)}) })
	}
	wg.Wait()
	if errs[0] != nil {
		errs[0], errs[1] = errs[1], errs[0]
	}
	require.NoError(t, errs[0], "one of two concurrent puts succeeds")
	require.ErrorIs(t, errs[1], driver.ErrConflict, "the other conflicts")
}
