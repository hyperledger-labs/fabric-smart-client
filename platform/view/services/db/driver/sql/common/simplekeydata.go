/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
)

func newSimpleKeyDataPersistence(writeDB *sql.DB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *simpleKeyDataPersistence {
	return &simpleKeyDataPersistence{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
	}
}

type simpleKeyDataPersistence struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      *sql.DB
	ci           Interpreter
}

func (db *simpleKeyDataPersistence) GetData(key string) ([]byte, error) {
	where, params := Where(db.ci.Cmp("key", "=", key))
	query := fmt.Sprintf("SELECT data FROM %s %s", db.table, where)
	logger.Debug(query, params)

	return QueryUnique[[]byte](db.readDB, query, params...)
}

func (db *simpleKeyDataPersistence) ExistData(key string) (bool, error) {
	data, err := db.GetData(key)
	return len(data) > 0, err
}

func (db *simpleKeyDataPersistence) PutData(key string, etx []byte) error {
	query := fmt.Sprintf("INSERT INTO %s (key, data) VALUES ($1, $2)", db.table)
	logger.Debugf(query, key, len(etx))
	_, err := db.writeDB.Exec(query, key, etx)
	if err != nil && errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.Warnf("Data [%s] already in db. Skipping...", key)
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "failed executing query [%s]", query)
	}
	logger.Debugf("Data [%s] registered", key)
	return nil
}

func (db *simpleKeyDataPersistence) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		key TEXT NOT NULL PRIMARY KEY,
		data BYTEA NOT NULL
	);`, db.table))
}
