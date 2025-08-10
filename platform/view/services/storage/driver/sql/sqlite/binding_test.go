/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
)

type mockIdentity string

func (m mockIdentity) UniqueID() string {
	return string(m)
}

func (m mockIdentity) IsNone() bool {
	return false
}

func TestPutBindings_MultipleEphemerals(t *testing.T) {
	ctx := context.Background()

	// Create mock DB and mock expectations
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	store := &BindingStore{
		writeDB: db,
		table:   "bindings",
	}

	// No GetLongTerm substitution: pass identity as-is
	// store.GetLongTerm = func(ctx context.Context, id view.Identity) (view.Identity, error) {
	// 	return nil, nil
	// }

	// Input identities
	longTerm := view.Identity("long")
	e1 := view.Identity("eph1")
	e2 := view.Identity("eph2")
	// e3 := view.Identity("eph3")

	// Expected SQL query
	expectedSQL := regexp.QuoteMeta(`INSERT INTO bindings (ephemeral_hash, long_term_id) VALUES ($1, $2), ($3, $4) ON CONFLICT DO NOTHING;`)

	mock.ExpectExec(expectedSQL).
		WithArgs(e1.UniqueID(), longTerm, e2.UniqueID(), longTerm).
		WillReturnResult(sqlmock.NewResult(1, 2))

	err = store.PutBindings(ctx, longTerm, e1, e2)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
