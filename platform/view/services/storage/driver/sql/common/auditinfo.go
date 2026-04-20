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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func NewAuditInfoStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ph sq.PlaceholderFormat) *AuditInfoStore {
	return &AuditInfoStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		sb:           sq.StatementBuilder.PlaceholderFormat(ph),
	}
}

type AuditInfoStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	sb           sq.StatementBuilderType
}

func (db *AuditInfoStore) GetAuditInfo(ctx context.Context, id view.Identity) ([]byte, error) {
	query, params, err := db.sb.Select("audit_info").
		From(db.table).
		Where(sq.Eq{"id": id.UniqueID()}).
		ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}

	logger.Debug(query, params)

	return QueryUniqueContext[[]byte](ctx, db.readDB, query, params...)
}

func (db *AuditInfoStore) PutAuditInfo(ctx context.Context, id view.Identity, info []byte) error {
	query, params, err := db.sb.Insert(db.table).
		Columns("id", "audit_info").
		Values(id.UniqueID(), info).
		ToSql()
	if err != nil {
		return errors.Wrapf(err, "failed to build query")
	}

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
