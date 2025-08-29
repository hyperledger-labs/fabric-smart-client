/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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

// func (db *BindingStore) PutBinding(ctx context.Context, longTerm view.Identity, ephemeral view.Identity) error {
// 	logger.DebugfContext(ctx, "put binding for pair [%s:%s]", ephemeral.UniqueID(), longTerm.UniqueID())
// 	if lt, err := db.GetLongTerm(ctx, longTerm); err != nil {
// 		return err
// 	} else if lt != nil && !lt.IsNone() {
// 		logger.DebugfContext(ctx, "replacing [%s] with long term [%s]", longTerm.UniqueID(), lt.UniqueID())
// 		longTerm = lt
// 	} else {
// 		logger.DebugfContext(ctx, "Id [%s] is an unregistered long term ID", longTerm.UniqueID())
// 	}
// 	query := fmt.Sprintf(`
// 		BEGIN;
// 		INSERT INTO %s (ephemeral_hash, long_term_id)
// 		VALUES ($1, $2)
// 		ON CONFLICT DO NOTHING;
// 		INSERT INTO %s (ephemeral_hash, long_term_id)
// 		VALUES ($3, $4)
// 		ON CONFLICT DO NOTHING;
// 		COMMIT;
// 		`, db.table, db.table)

// 	logger.Debug(query, ephemeral.UniqueID(), longTerm.UniqueID())
// 	_, err := db.writeDB.ExecContext(ctx, query, ephemeral.UniqueID(), longTerm, longTerm.UniqueID(), longTerm)
// 	if err == nil {
// 		logger.DebugfContext(ctx, "long-term and ephemeral ids registered [%s,%s]", longTerm, ephemeral)
// 		return nil
// 	}
// 	if errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
// 		logger.InfofContext(ctx, "tuple [%s,%s] already in db. Skipping...", ephemeral, longTerm)
// 		return nil
// 	}
// 	return errors.Wrapf(err, "failed executing query [%s]", query)
// }

func (db *BindingStore) PutBindings(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error {
	if len(ephemeral) == 0 {
		return errors.New("no ephemeral identities provided")
	}

	logger.DebugfContext(ctx, "put bindings for %d ephemeral(s) with long term [%s]", len(ephemeral), longTerm.UniqueID())

	// Resolve canonical long-term ID
	if lt, err := db.GetLongTerm(ctx, longTerm); err != nil {
		return err
	} else if lt != nil && !lt.IsNone() {
		logger.DebugfContext(ctx, "replacing [%s] with long term [%s]", longTerm.UniqueID(), lt.UniqueID())
		longTerm = lt
	} else {
		logger.DebugfContext(ctx, "Id [%s] is an unregistered long term ID", longTerm.UniqueID())
	}

	// Build single INSERT with multiple VALUES
	query := fmt.Sprintf(`INSERT INTO %s (ephemeral_hash, long_term_id) VALUES `, db.table)

	args := []interface{}{}
	argsReferences := []string{"($1, $2)"}
	args = append(args, longTerm.UniqueID(), longTerm)
	for i, eph := range ephemeral {
		args = append(args, eph.UniqueID(), longTerm)
		oneArgRef := fmt.Sprintf("($%d, $%d)", i*2+3, i*2+4)
		argsReferences = append(argsReferences, oneArgRef)
	}

	query += strings.Join(argsReferences, ", ")
	query += " ON CONFLICT DO NOTHING;"

	logger.DebugfContext(ctx, "executing bulk insert: %s", query)

	_, err := db.writeDB.ExecContext(ctx, query, args...)
	if err == nil {
		logger.DebugfContext(ctx, "long-term and ephemeral ids registered [%s,%s]", longTerm, ephemeral)
		return nil
	}
	if errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.InfofContext(ctx, "some tuples [%v, %s] already in db. Skipping...", ephemeral, longTerm)
		return nil
	}
	return errors.Wrapf(err, "failed executing query [%s]", query)
}
