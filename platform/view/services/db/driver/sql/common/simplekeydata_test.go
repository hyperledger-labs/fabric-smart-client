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
	. "github.com/onsi/gomega"
)

type (
	getFunc    func(*sql.DB, string) ([]byte, error)
	existsFunc func(*sql.DB, string) (bool, error)
	putFunc    func(*sql.DB, string, []byte) error
)

func GetData(t *testing.T, get getFunc) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"}).AddRow(data))

	result, err := get(db, key)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(Equal(data))
}

func GetData_NoData(t *testing.T, get getFunc) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "missing"
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"})) // no row

	result, err := get(db, key)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeNil())
}

func ExistData_True(t *testing.T, exists existsFunc) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"}).AddRow(data))

	result, err := exists(db, key)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeTrue())
}

func ExistData_False(t *testing.T, exist existsFunc) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "missing"
	query := regexp.QuoteMeta("SELECT data FROM test_table WHERE key = $1")

	mockDB.
		ExpectQuery(query).
		WithArgs(key).
		WillReturnRows(mockDB.NewRows([]string{"data"})) // empty result

	result, err := exist(db, key)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
	Expect(result).To(BeFalse())
}

func PutData_Success(t *testing.T, put putFunc) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("INSERT INTO test_table (key, data) VALUES ($1, $2) ON CONFLICT DO NOTHING")

	mockDB.
		ExpectExec(query).
		WithArgs(key, data).
		WillReturnResult(sqlmock.NewResult(1, 1)) // 1 row inserted

	err = put(db, key, data)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
}

func PutData_Conflict(t *testing.T, put putFunc) {
	RegisterTestingT(t)

	db, mockDB, err := sqlmock.New()
	Expect(err).ToNot(HaveOccurred())

	key := "key1"
	data := []byte("value1")
	query := regexp.QuoteMeta("INSERT INTO test_table (key, data) VALUES ($1, $2) ON CONFLICT DO NOTHING")

	mockDB.
		ExpectExec(query).
		WithArgs(key, data).
		WillReturnResult(sqlmock.NewResult(1, 0)) // conflict, 0 rows affected

	err = put(db, key, data)

	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
	Expect(err).ToNot(HaveOccurred())
}
