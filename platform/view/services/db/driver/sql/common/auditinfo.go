/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

func NewAuditInfoStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *AuditInfoStore {
	return &AuditInfoStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
	}
}

type AuditInfoStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	ci           Interpreter
}

func (db *AuditInfoStore) GetAuditInfo(id view.Identity) ([]byte, error) {
	where, params := Where(db.ci.Cmp("id", "=", id.UniqueID()))
	query := fmt.Sprintf("SELECT audit_info FROM %s %s", db.table, where)
	logger.Debug(query, params)

	return QueryUnique[[]byte](db.readDB, query, params...)
}

func (db *AuditInfoStore) PutAuditInfo(id view.Identity, info []byte) error {
	query := fmt.Sprintf("INSERT INTO %s (id, audit_info) VALUES ($1, $2)", db.table)
	logger.Debugf(query, id, info)
	_, err := db.writeDB.Exec(query, id.UniqueID(), info)
	if err != nil && errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.Warnf("Audit info [%s] already in db. Skipping...", id)
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "failed executing query [%s]", query)
	}
	logger.Debugf("Signer [%s] registered", id)
	return nil
}

func (db *AuditInfoStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT NOT NULL PRIMARY KEY,
		audit_info BYTEA NOT NULL
	);`, db.table))
}
