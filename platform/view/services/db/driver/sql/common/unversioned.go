/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type UnversionedPersistence struct {
	driver.BasePersistence[driver.UnversionedValue, driver.UnversionedRead]
	writeDB *sql.DB
	table   string
}

func NewUnversionedPersistence(base driver.BasePersistence[driver.UnversionedValue, driver.UnversionedRead], writeDB *sql.DB, table string) *UnversionedPersistence {
	return &UnversionedPersistence{BasePersistence: base, writeDB: writeDB, table: table}
}

func NewUnversionedReadScanner() *unversionedReadScanner { return &unversionedReadScanner{} }

func NewUnversionedValueScanner() *unversionedValueScanner { return &unversionedValueScanner{} }

func NewUnversioned(readDB *sql.DB, writeDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *UnversionedPersistence {
	base := NewBasePersistence[driver.UnversionedValue, driver.UnversionedRead](writeDB, readDB, table, &unversionedReadScanner{}, &unversionedValueScanner{}, errorWrapper, ci, writeDB.Begin)
	return NewUnversionedPersistence(base, base.writeDB, base.table)
}

type unversionedReadScanner struct{}

func (s *unversionedReadScanner) Columns() []string {
	return []string{"pkey", "val"}
}
func (s *unversionedReadScanner) ReadValue(txs scannable) (driver.UnversionedRead, error) {
	var r driver.UnversionedRead
	err := txs.Scan(&r.Key, &r.Raw)
	return r, err
}

type unversionedValueScanner struct{}

func (s *unversionedValueScanner) Columns() []string {
	return []string{"val"}
}
func (s *unversionedValueScanner) ReadValue(txs scannable) (driver.UnversionedValue, error) {
	var r driver.UnversionedValue
	err := txs.Scan(&r)
	return r, err
}

func (s *unversionedValueScanner) WriteValue(value driver.UnversionedValue) []any {
	return []any{value}
}

func (db *UnversionedPersistence) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (pkey, ns)
	);`, db.table))
}
