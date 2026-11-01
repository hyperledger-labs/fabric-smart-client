/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type provider[V any] func(name string) (V, error)

// TestCases runs the shared key-value store cases against unversionedProvider and,
// when unversionedNotifierProvider is non-nil, the notifier cases against it.
func TestCases(t *testing.T,
	unversionedProvider provider[driver.KeyValueStore],
	unversionedNotifierProvider provider[driver.UnversionedNotifier],
	baseUnpacker func(p driver.KeyValueStore) *KeyValueStore,
) {
	for _, c := range Cases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer func() { _ = un.Close() }()
			c.Fn(xt, un)
		})
	}
	for _, c := range UnversionedCases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer func() { _ = un.Close() }()
			c.Fn(xt, un)
		})
	}
	for _, c := range ErrorCases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		b := baseUnpacker(un)
		t.Run(c.Name, func(xt *testing.T) {
			defer func() { _ = un.Close() }()
			c.Fn(xt, b.readDB, b.writeDB, b.errorWrapper, b.table)
		})
	}
	if unversionedNotifierProvider != nil {
		for _, c := range UnversionedNotifierCases {
			un, err := unversionedNotifierProvider(c.Name)
			if err != nil {
				t.Fatal(err)
			}
			t.Run(c.Name, func(xt *testing.T) {
				defer func() { _ = un.Close() }()
				c.Fn(xt, un)
			})
		}
	}
}

func TestPutBindingsMultipleEphemeralsCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()

	// Input identities
	longTerm := view.Identity("long")
	e1 := view.Identity("eph1")
	e2 := view.Identity("eph2")

	// Check that store does not have bindings for e1 and e2
	lt, err := db.GetLongTerm(ctx, e1)
	require.NoError(t, err)
	require.ElementsMatch(t, len(lt), 0)
	lt, err = db.GetLongTerm(ctx, e2)
	require.NoError(t, err)
	require.ElementsMatch(t, len(lt), 0)

	// Create new bindings
	err = db.PutBindings(ctx, longTerm, e1, e2)
	require.NoError(t, err)

	// Check that the bindings where correctly written
	lt, err = db.GetLongTerm(ctx, e1)
	require.NoError(t, err)
	require.ElementsMatch(t, lt, longTerm)

	lt, err = db.GetLongTerm(ctx, e2)
	require.NoError(t, err)
	require.ElementsMatch(t, lt, longTerm)
}

func TestManyManyPutBindingsCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()

	// Input identities
	longTerm := view.Identity("long")
	e := []view.Identity{}
	for i := range BindingStoreMaxEphemerals + 1 {
		e = append(e, view.Identity("eph"+strconv.Itoa(i)))
	}
	err := db.PutBindings(ctx, longTerm, e...)
	require.Error(t, err)
}

// TestPutBindingsRefusesKnownLongTermAsEphemeralCommon checks that PutBindings refuses to bind
// an identity with a self-row as an ephemeral, and still writes the other ephemerals of the call.
func TestPutBindingsRefusesKnownLongTermAsEphemeralCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()
	alice, mallory := view.Identity("alice"), view.Identity("mallory")
	require.NoError(t, db.PutBindings(ctx, alice, view.Identity("alice-eph")))

	err := db.PutBindings(ctx, mallory, alice, view.Identity("mallory-eph"))
	require.ErrorIs(t, err, driver.ErrBindingRefused)

	lt, err := db.GetLongTerm(ctx, alice)
	require.NoError(t, err)
	require.Equal(t, alice, lt)
	lt, err = db.GetLongTerm(ctx, view.Identity("mallory-eph"))
	require.NoError(t, err)
	require.Equal(t, mallory, lt)
}

// TestPutBindingsConflictKeepsRowAndReturnsNilCommon checks that a conflicting bind keeps the
// existing row and returns nil.
func TestPutBindingsConflictKeepsRowAndReturnsNilCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()
	x := view.Identity("x")
	require.NoError(t, db.PutBindings(ctx, view.Identity("alice"), x))
	require.NoError(t, db.PutBindings(ctx, view.Identity("bob"), x))

	lt, err := db.GetLongTerm(ctx, x)
	require.NoError(t, err)
	require.Equal(t, view.Identity("alice"), lt)
}

// TestPutBindingsDoesNotFollowRemoteRedirectCommon checks that a redirect of a long-term
// identity cannot be stored, so later binds to that long term are not redirected.
func TestPutBindingsDoesNotFollowRemoteRedirectCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()
	alice, mallory, x := view.Identity("alice"), view.Identity("mallory"), view.Identity("x")
	require.NoError(t, db.PutBindings(ctx, alice, view.Identity("alice-eph")))
	require.ErrorIs(t, db.PutBindings(ctx, mallory, alice), driver.ErrBindingRefused)

	require.NoError(t, db.PutBindings(ctx, alice, x))
	lt, err := db.GetLongTerm(ctx, x)
	require.NoError(t, err)
	require.Equal(t, alice, lt)
}

// TestReplaceBindingReplacesPlantedRowCommon checks that ReplaceBinding overwrites an existing
// binding of the ephemeral and refuses an ephemeral with a self-row.
func TestReplaceBindingReplacesPlantedRowCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()
	alice, mallory, x := view.Identity("alice"), view.Identity("mallory"), view.Identity("x")
	require.NoError(t, db.PutBindings(ctx, mallory, x))

	require.NoError(t, db.ReplaceBinding(ctx, alice, x))
	lt, err := db.GetLongTerm(ctx, x)
	require.NoError(t, err)
	require.Equal(t, alice, lt)
	lt, err = db.GetLongTerm(ctx, alice)
	require.NoError(t, err)
	require.Equal(t, alice, lt)

	require.ErrorIs(t, db.ReplaceBinding(ctx, mallory, alice), driver.ErrBindingRefused)
	lt, err = db.GetLongTerm(ctx, alice)
	require.NoError(t, err)
	require.Equal(t, alice, lt)

	// a long term bound to another long term keeps that binding
	aliceMSP, y := view.Identity("alice-msp"), view.Identity("y")
	require.NoError(t, db.PutBindings(ctx, alice, aliceMSP))
	require.NoError(t, db.ReplaceBinding(ctx, aliceMSP, y))
	lt, err = db.GetLongTerm(ctx, aliceMSP)
	require.NoError(t, err)
	require.Equal(t, alice, lt)
}

// TestPutBindingsConcurrentSameEphemeralCommon checks that concurrent binds of one ephemeral to
// different long terms all return nil and leave exactly one of them bound.
func TestPutBindingsConcurrentSameEphemeralCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()
	x := view.Identity("x")
	longTerms := []view.Identity{view.Identity("alice"), view.Identity("bob")}

	var wg sync.WaitGroup
	errs := make([]error, len(longTerms))
	for i, lt := range longTerms {
		wg.Go(func() { errs[i] = db.PutBindings(ctx, lt, x) })
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	lt, err := db.GetLongTerm(ctx, x)
	require.NoError(t, err)
	require.Contains(t, longTerms, lt)
}

// TestPutBindingsUnknownLongTermIsFirstWriteWinsCommon pins the limit of unproven binds: an
// identity with neither a self-row nor a resolver can be bound as another party's ephemeral, and
// later binds to it then follow that binding.
func TestPutBindingsUnknownLongTermIsFirstWriteWinsCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()
	alice, mallory, x := view.Identity("alice"), view.Identity("mallory"), view.Identity("x")
	require.NoError(t, db.PutBindings(ctx, mallory, alice))

	require.NoError(t, db.PutBindings(ctx, alice, x))
	lt, err := db.GetLongTerm(ctx, x)
	require.NoError(t, err)
	require.Equal(t, mallory, lt)
}

// TestReplaceBindingDoesNotFollowPlantedRedirectCommon checks that ReplaceBinding binds the
// ephemeral to the given long term, not to the identity a planted row binds that long term to.
func TestReplaceBindingDoesNotFollowPlantedRedirectCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()
	alice, mallory, x := view.Identity("alice"), view.Identity("mallory"), view.Identity("x")
	require.NoError(t, db.PutBindings(ctx, mallory, alice))

	require.NoError(t, db.ReplaceBinding(ctx, alice, x))
	lt, err := db.GetLongTerm(ctx, x)
	require.NoError(t, err)
	require.Equal(t, alice, lt)
}

// TestReplaceBindingDoesNotRepairPlantedLongTermRowCommon pins that ReplaceBinding leaves a
// planted row on the long-term identity itself in place: it writes longTerm's self-row with
// ON CONFLICT DO NOTHING, so only the operator check of the upgrade guide removes such a row.
func TestReplaceBindingDoesNotRepairPlantedLongTermRowCommon(t *testing.T, db *BindingStore) {
	ctx := t.Context()
	alice, mallory, x := view.Identity("alice"), view.Identity("mallory"), view.Identity("x")
	require.NoError(t, db.PutBindings(ctx, mallory, alice))

	require.NoError(t, db.ReplaceBinding(ctx, alice, x))
	lt, err := db.GetLongTerm(ctx, alice)
	require.NoError(t, err)
	require.Equal(t, mallory, lt)
}
