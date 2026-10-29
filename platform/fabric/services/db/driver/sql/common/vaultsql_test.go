/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	dbsql "database/sql"
	sqldriver "database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/pagination"
)

// idKey binds keys unchanged, so the asserted args are the raw values.
func idKey(k driver.PKey) any { return k }

// isoMapper maps every isolation level to the default one. Only
// NewTxLockVaultReader's lazy reader would use it, and these tests never
// force that reader open.
type isoMapper struct{}

func (isoMapper) Map(driver.IsolationLevel) (dbsql.IsolationLevel, error) {
	return dbsql.LevelDefault, nil
}

// wrapError passes errors through unchanged.
type passthroughWrapper struct{}

func (passthroughWrapper) WrapError(err error) error { return err }

var vaultTables = common.VaultTables{StateTable: "state", StatusTable: "status"}

func newVaultMock(t *testing.T) (*common.VaultStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	store := common.NewVaultStore(db, db, vaultTables, passthroughWrapper{}, idKey, isoMapper{})
	return store, mockDB
}

func TestVaultSQL_NewTxLockVaultReader(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMock(t)

	mockDB.
		ExpectExec("INSERT INTO status (tx_id,code) VALUES ($1,$2) ON CONFLICT DO NOTHING").
		WithArgs("tx1", int64(driver.Busy)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	_, err := store.NewTxLockVaultReader(context.Background(), "tx1", driver.LevelDefault)
	require.NoError(t, err)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

func TestVaultSQL_SetStatuses(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMock(t)

	mockDB.
		ExpectExec("INSERT INTO status (tx_id,code,message) VALUES ($1,$2,$3),($4,$5,$6) "+
			"ON CONFLICT (tx_id) DO UPDATE SET code = EXCLUDED.code, message = EXCLUDED.message").
		WithArgs("tx1", int64(driver.Valid), "msg", "tx2", int64(driver.Valid), "msg").
		WillReturnResult(sqlmock.NewResult(1, 2))

	require.NoError(t, store.SetStatuses(context.Background(), driver.Valid, "msg", "tx1", "tx2"))
	require.NoError(t, mockDB.ExpectationsWereMet())
}

// TestVaultSQL_SetStatuses_WrapsError checks that a failed status write passes
// through the store's error wrapper, which maps a lost connection onto
// ErrConnectionLost for the committer to retry.
func TestVaultSQL_SetStatuses_WrapsError(t *testing.T) {
	t.Parallel()
	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)
	mockDB.ExpectExec("INSERT INTO status").WillReturnError(errors.New("connection reset by peer"))
	wrapper := &mock.SQLErrorWrapper{}
	wrapper.WrapErrorStub = func(err error) error { return errors.Wrap(dbdriver.ErrConnectionLost, err.Error()) }
	store := common.NewVaultStore(db, db, vaultTables, wrapper, idKey, isoMapper{})

	err = store.SetStatuses(context.Background(), driver.Invalid, "msg", "tx1")
	require.ErrorIs(t, err, dbdriver.ErrConnectionLost)
}

func TestVaultSQL_SetStatusesBusy(t *testing.T) {
	t.Parallel()
	store, _ := newVaultMock(t)

	query, args, err := store.SetStatusesBusy([]driver.TxID{"tx1", "tx2"})
	require.NoError(t, err)
	require.Equal(t, "INSERT INTO status (tx_id,code) VALUES ($1,$2),($3,$4) "+
		"ON CONFLICT (tx_id) DO UPDATE SET code = EXCLUDED.code", query)
	require.Equal(t, []any{"tx1", driver.Busy, "tx2", driver.Busy}, args)
}

func TestVaultSQL_UpsertStates(t *testing.T) {
	t.Parallel()
	store, _ := newVaultMock(t)

	// exactly one write, so map iteration order cannot make this flaky
	writes := driver.Writes{
		"ns": {"k1": driver.VaultValue{Raw: []byte("v"), Version: []byte("1")}},
	}
	query, args, err := store.UpsertStates(writes, driver.MetaWrites{})
	require.NoError(t, err)
	require.Equal(t, "INSERT INTO state (ns,pkey,val,kversion,metadata) VALUES ($1,$2,$3,$4,$5) "+
		"ON CONFLICT (ns,pkey) DO UPDATE SET val = EXCLUDED.val, kversion = EXCLUDED.kversion, metadata = EXCLUDED.metadata", query)
	require.Equal(t, []any{"ns", "k1", []byte("v"), []byte("1"), []byte{}}, args)
}

func TestVaultSQL_SetStatusesValid(t *testing.T) {
	t.Parallel()
	store, _ := newVaultMock(t)

	query, args, err := store.SetStatusesValid([]driver.TxID{"tx1", "tx2"})
	require.NoError(t, err)
	require.Equal(t, "UPDATE status SET message = '', code = $1 WHERE tx_id IN ($2,$3)", query)
	require.Equal(t, []any{driver.Valid, "tx1", "tx2"}, args)

	query, args, err = store.SetStatusesValid([]driver.TxID{"tx1"})
	require.NoError(t, err)
	require.Equal(t, "UPDATE status SET message = '', code = $1 WHERE tx_id IN ($2)", query)
	require.Equal(t, []any{driver.Valid, "tx1"}, args)
}

func TestVaultSQL_QueryState(t *testing.T) {
	t.Parallel()

	stateRows := func(mockDB sqlmock.Sqlmock) *sqlmock.Rows {
		return mockDB.NewRows([]string{"pkey", "kversion", "val"})
	}

	for _, tc := range []struct {
		name  string
		query string
		args  []sqldriver.Value
		run   func(store *common.VaultStore) error
	}{
		{
			name:  "GetStates two keys",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND pkey IN ($2,$3))",
			args:  []sqldriver.Value{"ns", "k1", "k2"},
			run: func(store *common.VaultStore) error {
				_, err := store.GetStates(context.Background(), "ns", "k1", "k2")
				return err
			},
		},
		{
			name:  "GetStates one key",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND pkey IN ($2))",
			args:  []sqldriver.Value{"ns", "k1"},
			run: func(store *common.VaultStore) error {
				_, err := store.GetStates(context.Background(), "ns", "k1")
				return err
			},
		},
		{
			name:  "GetStateRange both bounds",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND (pkey >= $2 AND pkey < $3)) ORDER BY pkey ASC",
			args:  []sqldriver.Value{"ns", "a", "z"},
			run: func(store *common.VaultStore) error {
				_, err := store.GetStateRange(context.Background(), "ns", "a", "z")
				return err
			},
		},
		{
			name:  "GetStateRange no bounds",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND (1=1)) ORDER BY pkey ASC",
			args:  []sqldriver.Value{"ns"},
			run: func(store *common.VaultStore) error {
				_, err := store.GetStateRange(context.Background(), "ns", "", "")
				return err
			},
		},
		{
			name:  "GetAllStates",
			query: "SELECT pkey, kversion, val FROM state WHERE ns = $1",
			args:  []sqldriver.Value{"ns"},
			run: func(store *common.VaultStore) error {
				_, err := store.GetAllStates(context.Background(), "ns")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, mockDB := newVaultMock(t)
			mockDB.ExpectQuery(tc.query).WithArgs(tc.args...).WillReturnRows(stateRows(mockDB))
			require.NoError(t, tc.run(store))
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestVaultSQL_GetStateMetadata(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMock(t)

	mockDB.
		ExpectQuery("SELECT metadata, kversion FROM state WHERE (ns = $1 AND pkey = $2)").
		WithArgs("ns", "k1").
		WillReturnRows(mockDB.NewRows([]string{"metadata", "kversion"}))

	_, _, err := store.GetStateMetadata(context.Background(), "ns", "k1")
	require.NoError(t, err)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

func TestVaultSQL_QueryStatus(t *testing.T) {
	t.Parallel()

	keysetFirst, err := pagination.KeysetWithField[string](0, 5, "tx_id", "TxID")
	require.NoError(t, err)
	keysetWithCursor, err := pagination.KeysetWithField[string](7, 5, "tx_id", "TxID")
	require.NoError(t, err)
	keysetWithCursor.FirstID = "tx3"
	keysetWithOffset, err := pagination.KeysetWithField[int](3, 5, "pos", "Pos")
	require.NoError(t, err)
	offset10, err := pagination.Offset(10, 5)
	require.NoError(t, err)
	offset0, err := pagination.Offset(0, 5)
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		query string
		args  []sqldriver.Value
		run   func(store *common.VaultStore) error
	}{
		{
			name:  "GetLast",
			query: "SELECT tx_id, code, message FROM status WHERE pos=(SELECT max(pos) FROM status WHERE code!=$1)",
			args:  []sqldriver.Value{int64(driver.Busy)},
			run: func(store *common.VaultStore) error {
				_, err := store.GetLast(context.Background())
				return err
			},
		},
		{
			name:  "GetTxStatus",
			query: "SELECT tx_id, code, message FROM status WHERE tx_id = $1",
			args:  []sqldriver.Value{"tx1"},
			run: func(store *common.VaultStore) error {
				_, err := store.GetTxStatus(context.Background(), "tx1")
				return err
			},
		},
		{
			name:  "GetTxStatuses",
			query: "SELECT tx_id, code, message FROM status WHERE tx_id IN ($1,$2)",
			args:  []sqldriver.Value{"tx1", "tx2"},
			run: func(store *common.VaultStore) error {
				_, err := store.GetTxStatuses(context.Background(), "tx1", "tx2")
				return err
			},
		},
		{
			name:  "GetAllTxStatuses none",
			query: "SELECT tx_id, code, message FROM status",
			run: func(store *common.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), pagination.None())
				return err
			},
		},
		{
			name:  "GetAllTxStatuses empty",
			query: "SELECT tx_id, code, message FROM status LIMIT 0",
			run: func(store *common.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), pagination.Empty())
				return err
			},
		},
		{
			name:  "GetAllTxStatuses offset with skip",
			query: "SELECT tx_id, code, message FROM status LIMIT 5 OFFSET 10",
			run: func(store *common.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), offset10)
				return err
			},
		},
		{
			name:  "GetAllTxStatuses offset without skip",
			query: "SELECT tx_id, code, message FROM status LIMIT 5",
			run: func(store *common.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), offset0)
				return err
			},
		},
		{
			name:  "GetAllTxStatuses keyset first page",
			query: "SELECT tx_id, code, message FROM status ORDER BY tx_id ASC LIMIT 5",
			run: func(store *common.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), keysetFirst)
				return err
			},
		},
		{
			name:  "GetAllTxStatuses keyset with cursor",
			query: "SELECT tx_id, code, message FROM status WHERE tx_id > $1 ORDER BY tx_id ASC LIMIT 5",
			args:  []sqldriver.Value{"tx3"},
			run: func(store *common.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), keysetWithCursor)
				return err
			},
		},
		{
			name:  "GetAllTxStatuses keyset with offset",
			query: "SELECT tx_id, code, message FROM status ORDER BY pos ASC LIMIT 5 OFFSET 3",
			run: func(store *common.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), keysetWithOffset)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, mockDB := newVaultMock(t)
			q := mockDB.ExpectQuery(tc.query)
			if len(tc.args) > 0 {
				q = q.WithArgs(tc.args...)
			}
			q.WillReturnRows(mockDB.NewRows([]string{"tx_id", "code", "message"}))
			require.NoError(t, tc.run(store))
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}
