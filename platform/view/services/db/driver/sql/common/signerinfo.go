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

func NewSignerInfoPersistence(writeDB *sql.DB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *SignerInfoPersistence {
	return &SignerInfoPersistence{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
	}
}

type SignerInfoPersistence struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      *sql.DB
	ci           Interpreter
}

func (db *SignerInfoPersistence) FilterExistingSigners(ids ...view.Identity) ([]view.Identity, error) {
	idHashes := make([]string, len(ids))
	inverseMap := make(map[string]view.Identity, len(ids))
	for i, id := range ids {
		idHash := id.UniqueID()
		idHashes[i] = idHash
		inverseMap[idHash] = id
	}
	where, params := Where(db.ci.InStrings("id", idHashes))
	query := fmt.Sprintf("SELECT id FROM %s %s", db.table, where)
	logger.Debug(query, params)

	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "error querying db")
	}

	existingSigners := make([]view.Identity, 0)
	for rows.Next() {
		var idHash string
		if err := rows.Scan(&idHash); err != nil {
			return nil, errors.Wrapf(err, "failed scanning row")
		}
		existingSigners = append(existingSigners, inverseMap[idHash])
	}
	logger.Debugf("Found %d out of %d signers", len(existingSigners), len(ids))
	return existingSigners, nil
}

func (db *SignerInfoPersistence) PutSigner(id view.Identity) error {
	query := fmt.Sprintf("INSERT INTO %s (id) VALUES ($1)", db.table)
	logger.Debugf(query, id)
	_, err := db.writeDB.Exec(query, id.UniqueID())
	if err != nil && errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.Warnf("Signer [%s] already in db. Skipping...", id)
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "failed executing query [%s]", query)
	}
	logger.Debugf("Signer [%s] registered", id)
	return nil
}

func (db *SignerInfoPersistence) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT NOT NULL PRIMARY KEY
	);`, db.table))
}
