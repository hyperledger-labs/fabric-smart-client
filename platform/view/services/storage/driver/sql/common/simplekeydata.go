/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"bytes"
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

// PutData stores data under key unless key already has a value, in which case
// it keeps the stored value and returns nil.
func (db *SimpleKeyDataStore) PutData(ctx context.Context, key string, data []byte) error {
	_, err := db.insert(ctx, key, data)
	return err
}

// PutDataStrict stores data under key. If key already has a value, it keeps
// the stored value and returns nil when that value equals data, or an error
// wrapping [driver.ErrConflict] when it differs.
//
// The insert and the read-back are separate statements. This is safe because
// the store never updates or deletes a row: once the insert finds the key
// taken, the value read back is the one that won.
func (db *SimpleKeyDataStore) PutDataStrict(ctx context.Context, key string, data []byte) error {
	inserted, err := db.insert(ctx, key, data)
	if err != nil || inserted {
		return err
	}
	stored, err := db.GetData(ctx, key)
	if err != nil {
		return errors.WithMessagef(err, "failed reading stored data for key [%s]", key)
	}
	return checkStored(key, stored, data)
}

// insert stores data under key unless key already has a value, and reports
// whether it did.
func (db *SimpleKeyDataStore) insert(ctx context.Context, key string, data []byte) (bool, error) {
	query, params := sqlbuild.New().
		WriteString("INSERT INTO " + db.table + " (key,data) VALUES ").
		WriteTuples([]sqlbuild.Tuple{{key, data}}).
		WriteString(" ON CONFLICT DO NOTHING").
		Build()
	logger.Debug(query, key)
	return execInsert(ctx, db.writeDB, db.errorWrapper, query, params...)
}

// execInsert runs an INSERT ... ON CONFLICT DO NOTHING and reports whether it
// inserted a row.
func execInsert(ctx context.Context, writeDB WriteDB, errorWrapper driver.SQLErrorWrapper, query string, params ...any) (bool, error) {
	result, err := writeDB.ExecContext(ctx, query, params...)
	if err != nil {
		return false, errors.Wrapf(errorWrapper.WrapError(err), "failed executing query [%s]", query)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, errors.Wrapf(errorWrapper.WrapError(err), "failed reading affected rows of query [%s]", query)
	}
	return rowsAffected > 0, nil
}

// checkStored returns nil when stored equals data, and an error wrapping
// [driver.ErrConflict] otherwise.
func checkStored(key string, stored, data []byte) error {
	if bytes.Equal(stored, data) {
		return nil
	}
	return errors.Wrapf(driver.ErrConflict, "key [%s] already holds a different value", key)
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
