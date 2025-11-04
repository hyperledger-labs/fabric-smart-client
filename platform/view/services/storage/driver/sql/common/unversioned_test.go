/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	. "github.com/onsi/gomega"
)

var (
	ns    = "namespace"
	key   = []byte("mykey")
	value = []byte("myvalue")
)

type dummyErrorWrapper struct{}

func (d *dummyErrorWrapper) WrapError(err error) error {
	return err
}

func TestGetState(t *testing.T) {
	RegisterTestingT(t)

	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	query := regexp.QuoteMeta("SELECT val FROM kv_table WHERE (ns = $1) AND (pkey = $2)")
	mock.ExpectQuery(query).WithArgs(ns, key).
		WillReturnRows(mock.NewRows([]string{"val"}).AddRow(value))

	store := mockKeyValueStore(db, db)
	result, err := store.GetState(context.Background(), ns, string(key))

	Expect(mock.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(driver.UnversionedValue(value)))
}

func TestGetState_QueryError(t *testing.T) {
	RegisterTestingT(t)

	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	query := regexp.QuoteMeta("SELECT val FROM kv_table WHERE (ns = $1) AND (pkey = $2)")
	mock.ExpectQuery(query).WithArgs(ns, key).WillReturnError(sql.ErrConnDone)

	store := mockKeyValueStore(db, db)
	_, err = store.GetState(context.Background(), ns, string(key))

	Expect(mock.ExpectationsWereMet()).To(Succeed())
	Expect(err).To(HaveOccurred())
}

func TestSetState(t *testing.T) {
	RegisterTestingT(t)

	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	query := regexp.QuoteMeta("INSERT INTO kv_table (ns, pkey, val) VALUES ($1, $2, $3)")
	mock.ExpectExec(query).WithArgs(ns, key, value).WillReturnResult(sqlmock.NewResult(1, 1))

	store := mockKeyValueStore(db, db)
	err = store.SetState(context.Background(), ns, string(key), value)

	Expect(mock.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
}

func TestSetState_ExecError(t *testing.T) {
	RegisterTestingT(t)

	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	query := regexp.QuoteMeta("INSERT INTO kv_table (ns, pkey, val) VALUES ($1, $2, $3)")
	mock.ExpectExec(query).WithArgs(ns, key, value).WillReturnError(sql.ErrConnDone)

	store := mockKeyValueStore(db, db)
	err = store.SetState(context.Background(), ns, string(key), value)

	Expect(mock.ExpectationsWereMet()).To(Succeed())
	Expect(err).To(HaveOccurred())
}

func TestDeleteState(t *testing.T) {
	RegisterTestingT(t)

	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	query := regexp.QuoteMeta("DELETE FROM kv_table WHERE (ns = $1) AND (pkey = $2)")
	mock.ExpectExec(query).WithArgs(ns, key).WillReturnResult(sqlmock.NewResult(1, 1))

	store := mockKeyValueStore(db, db)
	err = store.DeleteState(context.Background(), ns, string(key))

	Expect(mock.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
}

func TestDeleteState_ExecError(t *testing.T) {
	RegisterTestingT(t)

	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	query := regexp.QuoteMeta("DELETE FROM kv_table WHERE (ns = $1) AND (pkey = $2)")
	mock.ExpectExec(query).WithArgs(ns, key).WillReturnError(sql.ErrConnDone)

	store := mockKeyValueStore(db, db)
	err = store.DeleteState(context.Background(), ns, string(key))

	Expect(mock.ExpectationsWereMet()).To(Succeed())
	Expect(err).To(HaveOccurred())
}
func TestGetStateRangeScanIterator(t *testing.T) {
	RegisterTestingT(t)
	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	startKey := []byte("a")
	endKey := []byte("z")

	query := regexp.QuoteMeta("SELECT pkey, val FROM kv_table WHERE (ns = $1) AND ((pkey >= $2) AND (pkey < $3)) ORDER BY pkey ASC")
	mock.ExpectQuery(query).
		WithArgs(ns, startKey, endKey).
		WillReturnRows(sqlmock.NewRows([]string{"pkey", "val"}).AddRow("a", []byte("val1")).AddRow("b", []byte("val2")))

	store := mockKeyValueStore(db, db)
	iter, err := store.GetStateRangeScanIterator(context.Background(), ns, string(startKey), string(endKey))
	Expect(err).ToNot(HaveOccurred())

	var results []string
	for {
		val, err := iter.Next()
		if val == nil {
			break // Iterator is exhausted
		}
		Expect(err).ToNot(HaveOccurred())
		results = append(results, string(val.Raw))
	}
	Expect(results).To(ConsistOf("val1", "val2"))
	Expect(mock.ExpectationsWereMet()).To(Succeed())
}

func TestGetStateSetIterator(t *testing.T) {
	RegisterTestingT(t)
	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	keys := []string{"a", "b"}

	query := regexp.QuoteMeta("SELECT pkey, val FROM kv_table WHERE (ns = $1) AND ((pkey) IN (($2), ($3)))")
	mock.ExpectQuery(query).
		WithArgs(ns, []byte("a"), []byte("b")).
		WillReturnRows(sqlmock.NewRows([]string{"pkey", "val"}).AddRow("a", []byte("val1")).AddRow("b", []byte("val2")))

	store := mockKeyValueStore(db, db)
	iter, err := store.GetStateSetIterator(context.Background(), ns, keys...)
	Expect(err).ToNot(HaveOccurred())

	var results []string
	for {
		val, err := iter.Next()
		if val == nil {
			break // Iterator is exhausted
		}
		Expect(err).ToNot(HaveOccurred())
		results = append(results, string(val.Raw))
	}
	Expect(results).To(ConsistOf("val1", "val2"))
	Expect(mock.ExpectationsWereMet()).To(Succeed())
}

func TestExec(t *testing.T) {
	RegisterTestingT(t)
	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	query := "UPDATE kv_table SET val = $1 WHERE ns = $2 AND pkey = $3"
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(query)).
		WithArgs([]byte("v"), "ns", []byte("key")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	tx, err := db.Begin()
	Expect(err).ToNot(HaveOccurred())

	store := mockKeyValueStore(db, db)
	store.Txn = tx

	_, err = store.Exec(context.Background(), query, []byte("v"), "ns", []byte("key"))
	Expect(err).ToNot(HaveOccurred())
	Expect(mock.ExpectationsWereMet()).To(Succeed())
}

func TestStats(t *testing.T) {
	RegisterTestingT(t)
	db, _, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	store := mockKeyValueStore(db, db)
	stats := store.Stats()
	Expect(stats).ToNot(BeNil())
}

func TestClose(t *testing.T) {
	RegisterTestingT(t)

	db, mock, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	// expect a Close() call on the database
	mock.ExpectClose()

	store := mockKeyValueStore(db, db)
	err = store.Close()
	Expect(err).ToNot(HaveOccurred())
	Expect(mock.ExpectationsWereMet()).To(Succeed())
}

func mockKeyValueStore(write *sql.DB, read *sql.DB) *common2.KeyValueStore {
	return common2.NewKeyValueStore(write, read, "kv_table", &dummyErrorWrapper{}, sqlite.NewConditionInterpreter())
}
