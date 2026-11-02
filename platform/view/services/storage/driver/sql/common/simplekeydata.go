/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// NewSimpleKeyDataStore returns a simple key-data store over table, reading
// through readDB and writing through writeDB. Its queries use $N placeholders,
// which both SQLite and Postgres accept.
func NewSimpleKeyDataStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper) *SimpleKeyDataStore {
	return &SimpleKeyDataStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
	}
}

type SimpleKeyDataStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
}

func (db *SimpleKeyDataStore) GetData(ctx context.Context, key string) ([]byte, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT data FROM " + db.table).
		WriteWhere(sqlbuild.Eq("key", key)).
		Build()
	logger.Debug(query)
	data, err := QueryUniqueContext[[]byte](ctx, db.readDB, query, params...)
	return data, db.errorWrapper.WrapError(err)
}

func (db *SimpleKeyDataStore) ExistData(ctx context.Context, key string) (bool, error) {
	data, err := db.GetData(ctx, key)
	return len(data) > 0, err
}

func (db *SimpleKeyDataStore) PutData(ctx context.Context, key string, data []byte) error {
	query, params := sqlbuild.New().
		WriteString("INSERT INTO " + db.table + " (key,data) VALUES ").
		WriteTuples([]sqlbuild.Tuple{{key, data}}).
		WriteString(" ON CONFLICT DO NOTHING").
		Build()
	logger.Debug(query, string(data))
	result, err := db.writeDB.ExecContext(ctx, query, params...)
	if err != nil {
		return errors.Wrapf(db.errorWrapper.WrapError(err), "failed executing query [%s]", query)
	}

	if rowsAffected, err := result.RowsAffected(); err == nil && rowsAffected == 0 {
		logger.DebugfContext(ctx, "Entry for key [%s] was already in the database. Skipped", key)
	}
	logger.DebugfContext(ctx, "Data [%s] registered", key)
	return nil
}

func (db *SimpleKeyDataStore) CreateSchema() error {
	return InitSchema(db.writeDB, db.schema())
}

// StoreName returns the table name; it identifies the store in the versions table.
func (db *SimpleKeyDataStore) StoreName() string { return db.table }

// Steps returns the store's schema steps. Step 1 creates the table if it does not exist.
func (db *SimpleKeyDataStore) Steps() []migrate.Step {
	return []migrate.Step{{Version: 1, Name: "create table", SQL: []string{db.schema()}}}
}

func (db *SimpleKeyDataStore) schema() string {
	return fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		key TEXT NOT NULL PRIMARY KEY,
		data BYTEA NOT NULL
	);`, db.table)
}
