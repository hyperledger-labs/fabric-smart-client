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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/cond"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

func NewAuditInfoStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci common.CondInterpreter) *AuditInfoStore {
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
	ci           common.CondInterpreter
}

func (db *AuditInfoStore) GetAuditInfo(id view.Identity) ([]byte, error) {
	query, params := q.Select().FieldsByName("audit_info").
		From(q.Table(db.table)).
		Where(cond.Eq("id", id.UniqueID())).
		Format(db.ci, nil)
	logger.Debug(query, params)

	return QueryUnique[[]byte](db.readDB, query, params...)
}

func (db *AuditInfoStore) PutAuditInfo(id view.Identity, info []byte) error {
	query, params := q.InsertInto(db.table).
		Fields("id", "audit_info").
		Row(id.UniqueID(), info).
		Format()

	logger.Debug(query, params)
	_, err := db.writeDB.Exec(query, params...)
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
