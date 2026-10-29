/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

var (
	ns    = "namespace"
	key   = []byte("mykey")
	value = []byte("myvalue")
)

type dummyErrorWrapper struct{}

func (*dummyErrorWrapper) WrapError(err error) error {
	return err
}

func TestGetState(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	query := regexp.QuoteMeta("SELECT val FROM kv_table WHERE (ns = $1 AND pkey = $2)")
	mock.ExpectQuery(query).WithArgs(ns, key).
		WillReturnRows(mock.NewRows([]string{"val"}).AddRow(value))

	store := mockKeyValueStore(db, db)
	result, err := store.GetState(context.Background(), ns, string(key))

	require.NoError(t, mock.ExpectationsWereMet())
	require.NoError(t, err)
	require.Equal(t, value, result)
}

func TestGetState_QueryError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	query := regexp.QuoteMeta("SELECT val FROM kv_table WHERE (ns = $1 AND pkey = $2)")
	mock.ExpectQuery(query).WithArgs(ns, key).WillReturnError(sql.ErrConnDone)

	store := mockKeyValueStore(db, db)
	_, err = store.GetState(context.Background(), ns, string(key))

	require.NoError(t, mock.ExpectationsWereMet())
	require.Error(t, err)
}

func TestSetState(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	query := regexp.QuoteMeta("INSERT INTO kv_table (ns,pkey,val) VALUES ($1,$2,$3) ON CONFLICT (ns, pkey) DO UPDATE SET val = EXCLUDED.val")
	mock.ExpectExec(query).WithArgs(ns, key, value).WillReturnResult(sqlmock.NewResult(1, 1))

	store := mockKeyValueStore(db, db)
	err = store.SetState(context.Background(), ns, string(key), value)

	require.NoError(t, mock.ExpectationsWereMet())
	require.NoError(t, err)
}

func TestSetState_ExecError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	query := regexp.QuoteMeta("INSERT INTO kv_table (ns,pkey,val) VALUES ($1,$2,$3) ON CONFLICT (ns, pkey) DO UPDATE SET val = EXCLUDED.val")
	mock.ExpectExec(query).WithArgs(ns, key, value).WillReturnError(sql.ErrConnDone)

	store := mockKeyValueStore(db, db)
	err = store.SetState(context.Background(), ns, string(key), value)

	require.NoError(t, mock.ExpectationsWereMet())
	require.Error(t, err)
}

func TestDeleteState(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	query := regexp.QuoteMeta("DELETE FROM kv_table WHERE (ns = $1 AND pkey = $2)")
	mock.ExpectExec(query).WithArgs(ns, key).WillReturnResult(sqlmock.NewResult(1, 1))

	store := mockKeyValueStore(db, db)
	err = store.DeleteState(context.Background(), ns, string(key))

	require.NoError(t, mock.ExpectationsWereMet())
	require.NoError(t, err)
}

func TestDeleteState_ExecError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	query := regexp.QuoteMeta("DELETE FROM kv_table WHERE (ns = $1 AND pkey = $2)")
	mock.ExpectExec(query).WithArgs(ns, key).WillReturnError(sql.ErrConnDone)

	store := mockKeyValueStore(db, db)
	err = store.DeleteState(context.Background(), ns, string(key))

	require.NoError(t, mock.ExpectationsWereMet())
	require.Error(t, err)
}

func TestGetStateRangeScanIterator(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	startKey := []byte("a")
	endKey := []byte("z")

	query := regexp.QuoteMeta("SELECT pkey, val FROM kv_table WHERE (ns = $1 AND pkey >= $2 AND pkey < $3) ORDER BY pkey ASC")
	mock.ExpectQuery(query).
		WithArgs(ns, startKey, endKey).
		WillReturnRows(sqlmock.NewRows([]string{"pkey", "val"}).AddRow("a", []byte("val1")).AddRow("b", []byte("val2")))

	store := mockKeyValueStore(db, db)
	iter, err := store.GetStateRangeScanIterator(context.Background(), ns, string(startKey), string(endKey))
	require.NoError(t, err)

	reads, err := iterators.ReadAllValues(iter)
	require.NoError(t, err)
	require.ElementsMatch(t, []driver.UnversionedRead{{Key: "a", Raw: []byte("val1")}, {Key: "b", Raw: []byte("val2")}}, reads)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetStateSetIterator(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	keys := []string{"a", "b"}

	query := regexp.QuoteMeta("SELECT pkey, val FROM kv_table WHERE (ns = $1 AND pkey IN ($2,$3))")
	mock.ExpectQuery(query).
		WithArgs(ns, []byte("a"), []byte("b")).
		WillReturnRows(sqlmock.NewRows([]string{"pkey", "val"}).AddRow("a", []byte("val1")).AddRow("b", []byte("val2")))

	store := mockKeyValueStore(db, db)
	iter, err := store.GetStateSetIterator(context.Background(), ns, keys...)
	require.NoError(t, err)

	reads, err := iterators.ReadAllValues(iter)
	require.NoError(t, err)
	require.ElementsMatch(t, []driver.UnversionedRead{{Key: "a", Raw: []byte("val1")}, {Key: "b", Raw: []byte("val2")}}, reads)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExec(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	query := "UPDATE kv_table SET val = $1 WHERE ns = $2 AND pkey = $3"
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(query)).
		WithArgs([]byte("v"), "ns", []byte("key")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	tx, err := db.Begin()
	require.NoError(t, err)

	store := mockKeyValueStore(db, db)
	store.Txn = tx

	_, err = store.Exec(context.Background(), query, []byte("v"), "ns", []byte("key"))
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStats(t *testing.T) {
	t.Parallel()
	db, _, err := sqlmock.New()
	require.NoError(t, err)

	store := mockKeyValueStore(db, db)
	stats := store.Stats()
	require.NotNil(t, stats)
}

func TestClose(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	// expect a Close() call on the database
	mock.ExpectClose()

	store := mockKeyValueStore(db, db)
	err = store.Close()
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func mockKeyValueStore(write, read *sql.DB) *common.KeyValueStore {
	return common.NewKeyValueStore(write, read, "kv_table", &dummyErrorWrapper{})
}

// The range and set iterators pass query and row-read errors through the
// store's SQLErrorWrapper.
func TestKeyValueStoreIterators_WrapErrors(t *testing.T) {
	t.Parallel()

	iterate := map[string]func(s *common.KeyValueStore) error{
		"GetStateRangeScanIterator": func(s *common.KeyValueStore) error {
			it, err := s.GetStateRangeScanIterator(t.Context(), "ns", "a", "z")
			if err != nil {
				return err
			}
			_, err = iterators.ReadAllPointers(it)
			return err
		},
		"GetStateSetIterator": func(s *common.KeyValueStore) error {
			it, err := s.GetStateSetIterator(t.Context(), "ns", "k")
			if err != nil {
				return err
			}
			_, err = iterators.ReadAllPointers(it)
			return err
		},
	}
	for name, run := range iterate {
		for _, rowErr := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/rowErr=%t", name, rowErr), func(t *testing.T) {
				t.Parallel()
				db, sqlMock, err := sqlmock.New()
				require.NoError(t, err)
				t.Cleanup(func() { _ = db.Close() })
				dbErr := errors.New("connection reset by peer")
				if rowErr {
					sqlMock.ExpectQuery("SELECT").WillReturnRows(
						sqlmock.NewRows([]string{"pkey", "val"}).AddRow("a", []byte("v")).RowError(0, dbErr))
				} else {
					sqlMock.ExpectQuery("SELECT").WillReturnError(dbErr)
				}
				wrapper := &mock.SQLErrorWrapper{}
				wrapper.WrapErrorStub = func(err error) error { return errors.Wrap(driver.ErrConnectionLost, err.Error()) }

				err = run(common.NewKeyValueStore(db, db, "kv_table", wrapper))
				require.ErrorIs(t, err, driver.ErrConnectionLost)
				require.ErrorContains(t, err, dbErr.Error())
			})
		}
	}
}
