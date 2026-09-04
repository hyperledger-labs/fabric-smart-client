/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const BindingStoreMaxEphemerals = 1000

// NewBindingStore returns an identity binding store over table, reading through
// readDB and writing through writeDB. Its queries use $N placeholders, which
// both SQLite and Postgres accept.
func NewBindingStore(readDB *sql.DB, writeDB WriteDB, table string, errorWrapper driver.SQLErrorWrapper) *BindingStore {
	return &BindingStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
	}
}

type BindingStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
}

func (db *BindingStore) GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT long_term_id FROM " + db.table).
		WriteWhere(sqlbuild.Eq("ephemeral_hash", ephemeral.UniqueID())).
		Build()

	logger.Debug(query, params)
	result, err := QueryUniqueContext[view.Identity](ctx, db.readDB, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting wallet id for identity [%v]", ephemeral)
	}
	logger.DebugfContext(ctx, "found wallet id for identity [%v]: %v", ephemeral, result)
	return result, nil
}

func (db *BindingStore) HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT long_term_id FROM " + db.table).
		WriteWhere(sqlbuild.In("ephemeral_hash", this.UniqueID(), that.UniqueID())).
		Build()

	logger.Debug(query, params)
	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return false, errors.Wrapf(err, "error querying db")
	}
	defer utils.IgnoreErrorFunc(rows.Close)

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

func (db *BindingStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ephemeral_hash TEXT NOT NULL PRIMARY KEY,
		long_term_id BYTEA NOT NULL
	);`, db.table))
}

func (db *BindingStore) PutBindings(ctx context.Context, longTerm view.Identity, ephemerals ...view.Identity) error {
	if len(ephemerals) == 0 {
		return nil
	}
	if len(ephemerals) > BindingStoreMaxEphemerals {
		return errors.Errorf("Too many ephemerals (%d). Max allowed is %d", len(ephemerals), BindingStoreMaxEphemerals)
	}
	if longTerm == nil {
		return nil
	}

	logger.DebugfContext(ctx, "put bindings for %d ephemeral(s) with long term [%s]", len(ephemerals), longTerm.UniqueID())

	// Resolve canonical long-term ID
	if lt, err := db.GetLongTerm(ctx, longTerm); err != nil {
		return err
	} else if lt != nil && !lt.IsNone() {
		logger.DebugfContext(ctx, "replacing [%s] with long term [%s]", longTerm.UniqueID(), lt.UniqueID())
		longTerm = lt
	} else {
		logger.DebugfContext(ctx, "Id [%s] is an unregistered long term ID", longTerm.UniqueID())
	}

	// One INSERT with a row per identity, the long-term id bound to itself first.
	rows := make([]sqlbuild.Tuple, 0, len(ephemerals)+1)
	rows = append(rows, sqlbuild.Tuple{longTerm.UniqueID(), longTerm})
	for _, eph := range ephemerals {
		rows = append(rows, sqlbuild.Tuple{eph.UniqueID(), longTerm})
	}

	query, args := sqlbuild.New().
		WriteString("INSERT INTO " + db.table + " (ephemeral_hash, long_term_id) VALUES ").
		WriteTuples(rows).
		WriteString(" ON CONFLICT DO NOTHING;").
		Build()

	logger.DebugfContext(ctx, "executing bulk insert: %s", query)

	_, err := db.writeDB.ExecContext(ctx, query, args...)
	if err == nil {
		logger.DebugfContext(ctx, "long-term and ephemeral ids registered [%s,%s]", longTerm, ephemerals)
		return nil
	}
	if errors.Is(db.errorWrapper.WrapError(err), driver.UniqueKeyViolation) {
		logger.InfofContext(ctx, "some tuples [%v, %s] already in db. Skipping...", ephemerals, longTerm)
		return nil
	}
	return errors.Wrapf(err, "failed executing query [%s]", query)
}
