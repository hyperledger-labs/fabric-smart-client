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
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
)

func TestPutBindings_MultipleEphemerals(t *testing.T) {
	ctx := context.Background()

	// Create mock DB and mock expectations
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Wrap sqlmock's db into RWDB
	rwdb := &common3.RWDB{
		WriteDB: db,
		ReadDB:  db,
	}

	// Prepare table names
	tables := common2.TableNames{
		Binding: "bindings",
	}

	// Create store using constructor
	store, err := NewBindingStore(rwdb, tables)
	require.NoError(t, err)

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

	err = store.PutBindings(ctx, longTerm, e1, e2)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
