/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
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
	result, err := QueryUnique[view.Identity](db.readDB,
		fmt.Sprintf("SELECT long_term_id FROM %s %s", db.table, where),
		params...,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting wallet id for identity [%v]", ephemeral)
	}
	logger.Debugf("found wallet id for identity [%v]: %v", ephemeral, result)
	return result, nil
}
func (db *BindingPersistence) HaveSameBinding(this, that view.Identity) (bool, error) {
	where, params := Where(db.ci.InStrings("ephemeral_hash", []string{this.UniqueID(), that.UniqueID()}))
	query := fmt.Sprintf("SELECT long_term_id FROM %s %s", db.table, where)
	rows, err := db.readDB.Query(query, params...)
	if err != nil {
		return false, errors.Wrapf(err, "error querying db")
	}

	it := QueryIterator(rows, func(r RowScanner, id view.Identity) error { return r.Scan(&id) })
	longTermIds, err := collections.ReadAll(it)
	if err != nil {
		return false, err
	}
	if len(longTermIds) != 2 {
		return false, errors.Errorf("%d entries found instead of 2", len(longTermIds))
	}

	return longTermIds[0].Equal(longTermIds[1]), nil
}
func (db *BindingPersistence) PutBinding(ephemeral, longTerm view.Identity) error {
	query := fmt.Sprintf(
		"INSERT INTO %s (ephemeral_hash, long_term_id) "+
			"SELECT '%s', long_term_id FROM %s WHERE ephemeral_hash=$1",
		db.table, ephemeral.UniqueID(), db.table)

	if result, err := db.writeDB.Exec(query, longTerm.UniqueID()); err != nil && errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.Warnf("Tuple [%s,%s] already in db. Skipping...", ephemeral, longTerm)
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "failed executing query [%s]", query)
	} else if rowsAffected, err := result.RowsAffected(); err != nil {
		return errors.Wrapf(err, "failed fetching affected rows for query: %s", query)
	} else if rowsAffected > 1 {
		panic("unexpected result")
	} else if rowsAffected == 1 {
		logger.Debugf("New binding registered [%s:%s]", ephemeral, longTerm)
		return nil
	}

	logger.Debugf("Long term ID not seen before. Registering it as long term...")
	//query = fmt.Sprintf("INSERT INTO %s (ephemeral_hash, long_term_id) VALUES ($1, $2), ($3, $4)", db.table)
	//if _, err = db.writeDB.Exec(query, longTerm.UniqueID(), longTerm, ephemeral.UniqueID(), longTerm); err != nil {
	//	return errors.Wrapf(err, "failed inserting long-term id and ephemeral id")
	//}
	query = fmt.Sprintf("INSERT INTO %s (ephemeral_hash, long_term_id) VALUES ($1, $2)", db.table)
	if _, err := db.writeDB.Exec(query, longTerm.UniqueID(), longTerm); err != nil {
		return errors.Wrapf(err, "failed inserting long-term id and ephemeral id")
	}
	query = fmt.Sprintf("INSERT INTO %s (ephemeral_hash, long_term_id) VALUES ($1, $2)", db.table)
	if _, err := db.writeDB.Exec(query, ephemeral.UniqueID(), longTerm); err != nil {
		return errors.Wrapf(err, "failed inserting long-term id and ephemeral id")
	}
	logger.Infof("Long-term and ephemeral ids registered [%s,%s]", longTerm, ephemeral)
	return nil

}

func (db *BindingPersistence) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ephemeral_hash TEXT NOT NULL PRIMARY KEY,
		long_term_id BYTEA NOT NULL
	);`, db.table))
}
