/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

type (
	getFunc    func(*sql.DB, string) ([]byte, error)
	existsFunc func(*sql.DB, string) (bool, error)
	putFunc    func(*sql.DB, string, []byte) error
)

func GetData(t *testing.T, get getFunc) {
	t.Helper()

	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"}).AddRow(data))

	result, err := get(db, key)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
	require.Equal(t, data, result)
}

func GetDataNoData(t *testing.T, get getFunc) {
	t.Helper()

	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)

	key := "missing"
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"})) // no row

	result, err := get(db, key)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
	require.Nil(t, result)
}

func ExistDataTrue(t *testing.T, exists existsFunc) {
	t.Helper()

	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"}).AddRow(data))

	result, err := exists(db, key)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
	require.True(t, result)
}

func ExistDataFalse(t *testing.T, exist existsFunc) {
	t.Helper()

	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)

	key := "missing"
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"})) // empty result

	result, err := exist(db, key)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
	require.False(t, result)
}

func PutDataSuccess(t *testing.T, put putFunc) {
	t.Helper()

	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("INSERT INTO test_table (key,data) VALUES ($1,$2) ON CONFLICT DO NOTHING")

	mockDB.
		ExpectExec(query).
		WithArgs(key, data).
		WillReturnResult(sqlmock.NewResult(1, 1)) // 1 row inserted

	err = put(db, key, data)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
}

// PutDataConflict checks a first-write-wins put under a key that already holds
// a value: it keeps the stored value and returns nil.
func PutDataConflict(t *testing.T, put putFunc) {
	t.Helper()

	db, mockDB, err := sqlmock.New()
	require.NoError(t, err)

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("INSERT INTO test_table (key,data) VALUES ($1,$2) ON CONFLICT DO NOTHING")

	mockDB.
		ExpectExec(query).
		WithArgs(key, data).
		WillReturnResult(sqlmock.NewResult(1, 0)) // conflict, 0 rows affected

	err = put(db, key, data)

	require.NoError(t, mockDB.ExpectationsWereMet())
	require.NoError(t, err)
}

// PutDataStrictConflict checks a write-once put under a key that already holds
// a value: it returns nil when the stored value equals data, and an error
// wrapping driver.ErrConflict when it differs.
func PutDataStrictConflict(t *testing.T, put putFunc) {
	t.Helper()

	for name, tc := range map[string]struct {
		stored  []byte
		wantErr error
	}{
		"same value":      {stored: []byte("value1")},
		"different value": {stored: []byte("other"), wantErr: driver.ErrConflict},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			db, mockDB, err := sqlmock.New()
			require.NoError(t, err)

			key := "key1"
			data := []byte("value1")
			mockDB.
				ExpectExec(regexp.QuoteMeta("INSERT INTO test_table (key,data) VALUES ($1,$2) ON CONFLICT DO NOTHING")).
				WithArgs(key, data).
				WillReturnResult(sqlmock.NewResult(1, 0)) // key taken, 0 rows affected
			mockDB.
				ExpectQuery(regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")).
				WithArgs(key).
				WillReturnRows(mockDB.NewRows([]string{"data"}).AddRow(tc.stored))

			err = put(db, key, data)

			require.NoError(t, mockDB.ExpectationsWereMet())
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}
