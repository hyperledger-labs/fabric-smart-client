/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type BindingStore struct {
	*common2.BindingStore
	table        string
	writeDB      common2.WriteDB
	errorWrapper driver.SQLErrorWrapper
}

func NewBindingStore(dbs *common3.RWDB, tables common2.TableNames) (*BindingStore, error) {
	return newBindingStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.Binding), nil
}

func newBindingStore(readDB *sql.DB, writeDB common2.WriteDB, table string) *BindingStore {
	errorWrapper := &ErrorMapper{}
	return &BindingStore{
		BindingStore: common2.NewBindingStore(readDB, writeDB, table, errorWrapper, NewConditionInterpreter()),
		table:        table,
		writeDB:      writeDB,
		errorWrapper: errorWrapper,
	}
}
func (db *BindingStore) PutBinding(ctx context.Context, ephemeral, longTerm view.Identity) error {
	logger.DebugfContext(ctx, "Put binding for pair [%s:%s]", ephemeral.UniqueID(), longTerm.UniqueID())
	if lt, err := db.GetLongTerm(ctx, longTerm); err != nil {
		return err
	} else if lt != nil && !lt.IsNone() {
		logger.DebugfContext(ctx, "Replacing [%s] with long term [%s]", longTerm.UniqueID(), lt.UniqueID())
		longTerm = lt
	} else {
		logger.DebugfContext(ctx, "Id [%s] is an unregistered long term ID", longTerm.UniqueID())
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
	_, err := db.writeDB.ExecContext(ctx, query, ephemeral.UniqueID(), longTerm, longTerm.UniqueID(), longTerm)
	if err == nil {
		logger.DebugfContext(ctx, "Long-term and ephemeral ids registered [%s,%s]", longTerm, ephemeral)
		return nil
	}
	if errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.Warnf("Tuple [%s,%s] already in db. Skipping...", ephemeral, longTerm)
		return nil
	}
	return errors.Wrapf(err, "failed executing query [%s]", query)
}
