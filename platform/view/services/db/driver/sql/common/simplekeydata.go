/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	q "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	"github.com/pkg/errors"
)

func newSimpleKeyDataStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci common2.CondInterpreter) *simpleKeyDataStore {
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
	ci           common2.CondInterpreter
}

func (db *simpleKeyDataStore) GetData(key string) ([]byte, error) {
	query, params := q.Select().FieldsByName("data").
		From(q.Table(db.table)).
		Where(cond.Eq("key", key)).
		Format(db.ci)
	logger.Debug(query, params)

	return QueryUnique[[]byte](db.readDB, query, params...)
}

func (db *simpleKeyDataStore) ExistData(key string) (bool, error) {
	data, err := db.GetData(key)
	return len(data) > 0, err
}

func (db *simpleKeyDataStore) PutData(key string, data []byte) error {
	query, params := q.InsertInto(db.table).
		Fields("key", "data").
		Row(key, data).
		OnConflictDoNothing().
		Format()
	logger.Debug(query, params)
	result, err := db.writeDB.Exec(query, params...)
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
