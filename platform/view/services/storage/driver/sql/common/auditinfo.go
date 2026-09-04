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

type AuditInfoStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
}

func (db *AuditInfoStore) GetAuditInfo(ctx context.Context, id view.Identity) ([]byte, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT audit_info FROM " + db.table).
		WriteWhere(sqlbuild.Eq("id", id.UniqueID())).
		Build()

	logger.Debug(query, params)

	return QueryUniqueContext[[]byte](ctx, db.readDB, query, params...)
}

func (db *AuditInfoStore) PutAuditInfo(ctx context.Context, id view.Identity, info []byte) error {
	query, params := sqlbuild.New().
		WriteString("INSERT INTO " + db.table + " (id,audit_info) VALUES ").
		WriteTuples([]sqlbuild.Tuple{{id.UniqueID(), info}}).
		Build()

	logger.Debug(query, params)
	_, execErr := db.writeDB.ExecContext(ctx, query, params...)
	if execErr != nil && errors.Is(db.errorWrapper.WrapError(execErr), driver.UniqueKeyViolation) {
		logger.Infof("Audit info [%s] already in db. Skipping...", id)
		return nil
	}
	if execErr != nil {
		return errors.Wrapf(execErr, "failed executing query [%s]", query)
	}
	logger.DebugfContext(ctx, "signer [%s] registered", id)
	return nil
}

func (db *AuditInfoStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT NOT NULL PRIMARY KEY,
		audit_info BYTEA NOT NULL
	);`, db.table))
}
