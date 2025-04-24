/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type BindingStore struct {
	*common.BindingStore
	table        string
	writeDB      *sql.DB
	errorWrapper driver.SQLErrorWrapper
}

func NewBindingStore(opts Opts) (*BindingStore, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newBindingStore(dbs.ReadDB, dbs.WriteDB, tables.Binding), nil
}

func newBindingStore(readDB, writeDB *sql.DB, table string) *BindingStore {
	errorWrapper := &errorMapper{}
	return &BindingStore{
		BindingStore: common.NewBindingStore(readDB, writeDB, table, errorWrapper, NewInterpreter()),
		table:        table,
		writeDB:      writeDB,
		errorWrapper: errorWrapper,
	}
}
func (db *BindingStore) PutBinding(ephemeral, longTerm view.Identity) error {
	logger.Debugf("Put binding for pair [%s:%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	if lt, err := db.GetLongTerm(longTerm); err != nil {
		return err
	} else if lt != nil && !lt.IsNone() {
		logger.Debugf("Replacing [%s] with long term [%s]", longTerm.UniqueID(), lt.UniqueID())
		longTerm = lt
	} else {
		logger.Debugf("Id [%s] is an unregistered long term ID", longTerm.UniqueID())
	}
	query := fmt.Sprintf(`
		INSERT INTO %s (ephemeral_hash, long_term_id)
		VALUES ($1, $2), ($3, $4)
		ON CONFLICT DO NOTHING
		`, db.table)
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
