/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"strconv"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type provider[V any] func(name string) (V, error)

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
			defer utils.IgnoreErrorFunc(un.Close)
			c.Fn(xt, un)
		})
	}
	for _, c := range UnversionedCases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(un.Close)
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
			defer utils.IgnoreErrorFunc(un.Close)
			c.Fn(xt, b.readDB, b.writeDB, b.errorWrapper, b.table)
		})
	}
	for _, c := range UnversionedNotifierCases {
		un, err := unversionedNotifierProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(un.Close)
			c.Fn(xt, un)
		})
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
	for i := 0; i < BindingStoreMaxEphemerals+1; i++ {
		e = append(e, view.Identity("eph"+strconv.Itoa(i)))
	}
	err := db.PutBindings(ctx, longTerm, e...)
	require.Error(t, err)
}
