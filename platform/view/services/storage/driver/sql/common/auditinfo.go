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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// NewAuditInfoStore returns an audit-info store over table, reading through
// readDB and writing through writeDB. Its queries use $N placeholders, which
// both SQLite and Postgres accept.
func NewAuditInfoStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper) *AuditInfoStore {
	return &AuditInfoStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
	}
}

// AuditInfoStore is a write-once store of the audit info of identities.
type AuditInfoStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
}

// GetAuditInfo returns the audit info of id, or nil when there is none.
func (db *AuditInfoStore) GetAuditInfo(ctx context.Context, id view.Identity) ([]byte, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT audit_info FROM " + db.table).
		WriteWhere(sqlbuild.Eq("id", id.UniqueID())).
		Build()

	logger.Debug(query, params)

	info, err := QueryUniqueContext[[]byte](ctx, db.readDB, query, params...)
	return info, db.errorWrapper.WrapError(err)
}

// PutAuditInfo stores info as the audit info of id. If id already has audit
// info, it keeps the stored value and returns nil when that value equals info,
// or an error wrapping [driver.ErrConflict] when it differs. It reads the
// stored value back after the insert, which is safe because the store never
// updates or deletes a row.
func (db *AuditInfoStore) PutAuditInfo(ctx context.Context, id view.Identity, info []byte) error {
	query, params := sqlbuild.New().
		WriteString("INSERT INTO " + db.table + " (id,audit_info) VALUES ").
		WriteTuples([]sqlbuild.Tuple{{id.UniqueID(), info}}).
		WriteString(" ON CONFLICT DO NOTHING").
		Build()

	logger.Debug(query, params)
	inserted, err := execInsert(ctx, db.writeDB, db.errorWrapper, query, params...)
	if err != nil || inserted {
		return err
	}
	stored, err := db.GetAuditInfo(ctx, id)
	if err != nil {
		return errors.WithMessagef(err, "failed reading stored audit info of [%s]", id)
	}
	return checkStored(id.UniqueID(), stored, info)
}

func (db *AuditInfoStore) CreateSchema() error {
	return InitSchema(db.writeDB, db.schema())
}

// StoreName returns the table name; it identifies the store in the versions table.
func (db *AuditInfoStore) StoreName() string { return db.table }

// Steps returns the store's schema steps. Step 1 creates the table if it does not exist.
func (db *AuditInfoStore) Steps() []migrate.Step {
	return []migrate.Step{{Version: 1, Name: "create table", SQL: []string{db.schema()}}}
}

func (db *AuditInfoStore) schema() string {
	return fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT NOT NULL PRIMARY KEY,
		audit_info BYTEA NOT NULL
	);`, db.table)
}
