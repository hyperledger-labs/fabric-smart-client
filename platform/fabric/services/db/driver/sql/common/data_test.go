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
