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

func newSimpleKeyDataStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *simpleKeyDataStore {
	return &simpleKeyDataStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
	}
}

type simpleKeyDataStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	ci           Interpreter
}

func (db *simpleKeyDataStore) GetData(key string) ([]byte, error) {
	where, params := Where(db.ci.Cmp("key", "=", key))
	query := fmt.Sprintf("SELECT data FROM %s %s", db.table, where)
	logger.Debug(query, params)

	return QueryUnique[[]byte](db.readDB, query, params...)
}

func (db *simpleKeyDataStore) ExistData(key string) (bool, error) {
	data, err := db.GetData(key)
	return len(data) > 0, err
}

func (db *simpleKeyDataStore) PutData(key string, data []byte) error {
	query := fmt.Sprintf("INSERT INTO %s (key, data) VALUES ($1, $2) ON CONFLICT DO NOTHING", db.table)
	logger.Debug(query, key, len(data))
	result, err := db.writeDB.Exec(query, key, data)
	if err != nil {
		return errors.Wrapf(err, "failed executing query [%s]", query)
	}

	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		logger.Debugf("Entry for key [%s] was already in the database. Skipped", key)
	}
	logger.Debugf("Data [%s] registered", key)
	return nil
}

func (db *simpleKeyDataStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		key TEXT NOT NULL PRIMARY KEY,
		data BYTEA NOT NULL
	);`, db.table))
}
