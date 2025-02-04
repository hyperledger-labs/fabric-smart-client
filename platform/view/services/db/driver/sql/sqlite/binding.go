/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type BindingPersistence struct {
	*common.BindingPersistence
	table        string
	writeDB      common.WriteDB
	errorWrapper driver.SQLErrorWrapper
}

func NewBindingPersistence(opts common.Opts, table string) (*BindingPersistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newBindingPersistence(readDB, newRetryWriteDB(writeDB), table), nil
}

func newBindingPersistence(readDB *sql.DB, writeDB common.WriteDB, table string) *BindingPersistence {
	errorWrapper := &errorMapper{}
	return &BindingPersistence{
		BindingPersistence: common.NewBindingPersistence(readDB, writeDB, table, errorWrapper, NewInterpreter()),
		table:              table,
		writeDB:            writeDB,
		errorWrapper:       errorWrapper,
	}
}
func (db *BindingPersistence) PutBinding(ephemeral, longTerm view.Identity) error {
	logger.Debugf("Put binding for pair [%s:%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	if lt, err := db.GetLongTerm(longTerm); err != nil {
		return err
	} else if lt != nil && !lt.IsNone() {
		logger.Debugf("Replacing [%s] with long term [%s]", longTerm.UniqueID(), lt.UniqueID())
		longTerm = lt
	} else {
		logger.Infof("Id [%s] is an unregistered long term ID", longTerm.UniqueID())
	}
	query := fmt.Sprintf(`
		BEGIN;
		INSERT INTO %s (ephemeral_hash, long_term_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING;
		INSERT INTO %s (ephemeral_hash, long_term_id)
		VALUES ($3, $4)
		ON CONFLICT DO NOTHING;
		COMMIT;
		`, db.table, db.table)

	logger.Debug(query, ephemeral.UniqueID(), longTerm.UniqueID())
	_, err := db.writeDB.Exec(query, ephemeral.UniqueID(), longTerm, longTerm.UniqueID(), longTerm)
	if err == nil {
		logger.Debugf("Long-term and ephemeral ids registered [%s,%s]", longTerm, ephemeral)
		return nil
	}
	if errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.Warnf("Tuple [%s,%s] already in db. Skipping...", ephemeral, longTerm)
		return nil
	}
	return errors.Wrapf(err, "failed executing query [%s]", query)
}
