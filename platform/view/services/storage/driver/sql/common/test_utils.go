/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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
	baseUnpacker func(p driver.KeyValueStore) *KeyValueStore) {
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

func PutBindings(t *testing.T, store driver.BindingStore, mock sqlmock.Sqlmock) {
	ctx := context.Background()

	// Input identities
	longTerm := view.Identity("long")
	e1 := view.Identity("eph1")
	e2 := view.Identity("eph2")

	// Expected SQL query

	expectedSQL := regexp.QuoteMeta(`SELECT long_term_id FROM bindings WHERE ephemeral_hash = $1`)
	mock.ExpectQuery(expectedSQL).
		WithArgs(longTerm.UniqueID()).
		WillReturnRows(sqlmock.NewRows([]string{"long_term_id"})) // empty rows = no results

	expectedSQL = regexp.QuoteMeta(`INSERT INTO bindings (ephemeral_hash, long_term_id) VALUES ($1, $2), ($3, $4), ($5, $6) ON CONFLICT DO NOTHING;`)
	mock.ExpectExec(expectedSQL).
		WithArgs(longTerm.UniqueID(), longTerm, e1.UniqueID(), longTerm, e2.UniqueID(), longTerm).
		WillReturnResult(sqlmock.NewResult(1, 2))

	err := store.PutBindings(ctx, longTerm, e1, e2)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
