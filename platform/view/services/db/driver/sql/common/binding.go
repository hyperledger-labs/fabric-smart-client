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

func NewBindingPersistence(writeDB *sql.DB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ci Interpreter) *BindingPersistence {
	return &BindingPersistence{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		ci:           ci,
	}
}

type BindingPersistence struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      *sql.DB
	ci           Interpreter
}

func (db *BindingPersistence) GetLongTerm(ephemeral view.Identity) (view.Identity, error) {
	where, params := Where(db.ci.Cmp("ephemeral_hash", "=", ephemeral.UniqueID()))
	query := fmt.Sprintf("SELECT long_term_id FROM %s %s", db.table, where)
	logger.Info(query, params)
	result, err := QueryUnique[view.Identity](db.readDB, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting wallet id for identity [%v]", ephemeral)
	}
	logger.Infof("found wallet id for identity [%v]: %v", ephemeral, result)
	return result, nil
}
func (db *BindingPersistence) HaveSameBinding(this, that view.Identity) (bool, error) {
	where, params := Where(db.ci.InStrings("ephemeral_hash", []string{this.UniqueID(), that.UniqueID()}))
	query := fmt.Sprintf("SELECT long_term_id FROM %s %s", db.table, where)
	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return false, errors.Wrapf(err, "error querying db")
	}
	defer rows.Close()

	longTermIds := make([]view.Identity, 0, 2)
	for rows.Next() {
		var longTerm view.Identity
		if err := rows.Scan(&longTerm); err != nil {
			return false, err
		}
		longTermIds = append(longTermIds, longTerm)
	}
	if len(longTermIds) != 2 {
		return false, errors.Errorf("%d entries found instead of 2", len(longTermIds))
	}

	return longTermIds[0].Equal(longTermIds[1]), nil
}

func (db *BindingPersistence) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ephemeral_hash TEXT NOT NULL PRIMARY KEY,
		long_term_id BYTEA NOT NULL
	);`, db.table))
}
