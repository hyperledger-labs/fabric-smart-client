/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func NewSimpleKeyDataStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ph sq.PlaceholderFormat) *SimpleKeyDataStore {
	return &SimpleKeyDataStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		sb:           sq.StatementBuilder.PlaceholderFormat(ph),
	}
}

type SimpleKeyDataStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	sb           sq.StatementBuilderType
}

func (db *SimpleKeyDataStore) GetData(ctx context.Context, key string) ([]byte, error) {
	query, params, err := db.sb.Select("data").From(db.table).Where(sq.Eq{"key": key}).ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}
	logger.Debug(query)
	return QueryUniqueContext[[]byte](ctx, db.readDB, query, params...)
}

func (db *SimpleKeyDataStore) ExistData(ctx context.Context, key string) (bool, error) {
	data, err := db.GetData(ctx, key)
	return len(data) > 0, err
}

func (db *SimpleKeyDataStore) PutData(ctx context.Context, key string, data []byte) error {
	query, params, err := db.sb.Insert(db.table).
		Columns("key", "data").
		Values(key, data).
		Suffix("ON CONFLICT DO NOTHING").
		ToSql()
	if err != nil {
		return errors.Wrapf(err, "failed to build query")
	}
	logger.Debug(query, string(data))
	result, err := db.writeDB.ExecContext(ctx, query, params...)
	if err != nil {
		return errors.Wrapf(err, "failed executing query [%s]", query)
	}

	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		logger.DebugfContext(ctx, "Entry for key [%s] was already in the database. Skipped", key)
	}
	logger.DebugfContext(ctx, "Data [%s] registered", key)
	return nil
}

func (db *SimpleKeyDataStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		key TEXT NOT NULL PRIMARY KEY,
		data BYTEA NOT NULL
	);`, db.table))
}
