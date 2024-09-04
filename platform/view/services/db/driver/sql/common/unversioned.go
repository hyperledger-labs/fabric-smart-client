/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
)

type UnversionedPersistence struct {
	*basePersistence[driver.UnversionedValue, driver.UnversionedRead]
}

func NewUnversioned(readDB *sql.DB, writeDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *UnversionedPersistence {
	return &UnversionedPersistence{
		basePersistence: &basePersistence[driver.UnversionedValue, driver.UnversionedRead]{
			BaseDB:       common.NewBaseDB[*sql.Tx](func() (*sql.Tx, error) { return writeDB.Begin() }),
			writeDB:      writeDB,
			readDB:       readDB,
			table:        table,
			readScanner:  &unversionedReadScanner{},
			valueScanner: &unversionedValueScanner{},
			errorWrapper: errorWrapper,
			ci:           ci,
		},
	}
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
	return db.createSchema(fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey BYTEA NOT NULL,
		val BYTEA NOT NULL DEFAULT '',
		PRIMARY KEY (pkey, ns)
	);`, db.table))
}
