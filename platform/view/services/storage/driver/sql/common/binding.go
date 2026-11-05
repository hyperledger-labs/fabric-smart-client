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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const BindingStoreMaxEphemerals = 1000

// NewBindingStore returns an identity binding store over table, reading through
// readDB and writing through writeDB. Its queries use $N placeholders, which
// both SQLite and Postgres accept. timeout bounds each statement.
func NewBindingStore(readDB *sql.DB, writeDB WriteDB, table string, errorWrapper driver.SQLErrorWrapper, timeout QueryTimeout) *BindingStore {
	return &BindingStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		timeout:      timeout,
	}
}

type BindingStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	timeout      QueryTimeout
}

func (db *BindingStore) GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT long_term_id FROM " + db.table).
		WriteWhere(sqlbuild.Eq("ephemeral_hash", ephemeral.UniqueID())).
		Build()

	logger.Debug(query, params)
	ctx, cancel := db.timeout.Context(ctx)
	defer cancel()
	result, err := QueryUniqueContext[view.Identity](ctx, db.readDB, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting wallet id for identity [%v]", ephemeral)
	}
	logger.Debugf("found wallet id for identity [%v]: %v", ephemeral, result)
	return result, nil
}

func (db *BindingStore) HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error) {
	query, params := sqlbuild.New().
		WriteString("SELECT long_term_id FROM " + db.table).
		WriteWhere(sqlbuild.In("ephemeral_hash", this.UniqueID(), that.UniqueID())).
		Build()

	logger.Debug(query, params)
	ctx, cancel := db.timeout.Context(ctx)
	defer cancel()
	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return false, errors.Wrap(err, "error querying db")
	}
	defer func() { _ = rows.Close() }()

	longTermIDs := make([]view.Identity, 0, 2)
	for rows.Next() {
		var longTerm view.Identity
		if err := rows.Scan(&longTerm); err != nil {
			return false, err
		}
		longTermIDs = append(longTermIDs, longTerm)
	}
	if err := rows.Err(); err != nil {
		return false, errors.Wrap(err, "error iterating rows")
	}
	if len(longTermIDs) != 2 {
		return false, errors.Errorf("%d entries found instead of 2", len(longTermIDs))
	}

	return longTermIDs[0].Equal(longTermIDs[1]), nil
}

func (db *BindingStore) CreateSchema() error {
	return InitSchema(db.writeDB, db.schema())
}

// StoreName returns the table name; it identifies the store in the versions table.
func (db *BindingStore) StoreName() string { return db.table }

// Steps returns the store's schema steps. Step 1 creates the table if it does not exist.
func (db *BindingStore) Steps() []migrate.Step {
	return []migrate.Step{{Version: 1, Name: "create table", SQL: []string{db.schema()}}}
}

func (db *BindingStore) schema() string {
	return fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		ephemeral_hash TEXT NOT NULL PRIMARY KEY,
		long_term_id BYTEA NOT NULL
	);`, db.table)
}

// PutBindings binds each ephemeral to longTerm, and longTerm to itself, the self-row that marks
// it as a long-term identity. If longTerm is itself bound to an identity that has a self-row,
// the ephemerals are bound to that identity instead. An identity with neither a self-row nor a
// binding is stored as given, so the first bind of an unknown identity decides where it resolves.
//
// An ephemeral that has a self-row is a long-term identity, and binding it would redirect every
// lookup of that identity to longTerm, so PutBindings writes no row for it and returns an error
// wrapping driver.ErrBindingRefused; the other ephemerals of the call are still bound. An
// ephemeral already bound to a different long-term identity keeps its existing binding, and the
// conflict is not an error. Callers that need to tell a conflict from a successful bind read
// GetLongTerm afterwards.
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

	logger.Debugf("put bindings for %d ephemeral(s) with long term [%s]", len(ephemerals), longTerm.UniqueID())

	longTerm, err := db.canonicalLongTerm(ctx, longTerm)
	if err != nil {
		return err
	}
	allowed, refused, err := db.splitRefused(ctx, longTerm, ephemerals)
	if err != nil {
		return err
	}

	// One INSERT with a row per identity, the long-term id bound to itself first.
	rows := make([]sqlbuild.Tuple, 0, len(allowed)+1)
	rows = append(rows, sqlbuild.Tuple{longTerm.UniqueID(), longTerm})
	for _, eph := range allowed {
		rows = append(rows, sqlbuild.Tuple{eph.UniqueID(), longTerm})
	}

	query, args := sqlbuild.New().
		WriteString("INSERT INTO " + db.table + " (ephemeral_hash, long_term_id) VALUES ").
		WriteTuples(rows).
		WriteString(" ON CONFLICT DO NOTHING;").
		Build()

	logger.Debugf("executing bulk insert: %s", query)
	ctx, cancel := db.timeout.Context(ctx)
	defer cancel()

	if _, err := db.writeDB.ExecContext(ctx, query, args...); err != nil {
		err = db.errorWrapper.WrapError(err)
		if !errors.Is(err, driver.UniqueKeyViolation) {
			return errors.Wrapf(err, "failed executing query [%s]", query)
		}
	}
	return refusedError(longTerm, refused)
}

// ReplaceBinding binds ephemeral to exactly longTerm, replacing any existing binding of
// ephemeral, and binds longTerm to itself unless longTerm is already bound. Unlike PutBindings,
// it does not resolve longTerm through its own binding: the caller has proven that longTerm owns
// ephemeral, and a row binding longTerm elsewhere may have been planted by another party. It
// refuses an ephemeral that has a self-row, like PutBindings, and returns an error wrapping
// driver.ErrBindingRefused without writing anything.
func (db *BindingStore) ReplaceBinding(ctx context.Context, longTerm, ephemeral view.Identity) error {
	if longTerm.IsNone() || ephemeral.IsNone() {
		return nil
	}
	allowed, refused, err := db.splitRefused(ctx, longTerm, []view.Identity{ephemeral})
	if err != nil {
		return err
	}
	if err := refusedError(longTerm, refused); err != nil || len(allowed) == 0 {
		return err
	}

	// One deadline for the transaction, used by every statement in it: a deadline
	// on BeginTx alone does not interrupt a statement run under another context.
	ctx, cancel := db.timeout.Context(ctx)
	defer cancel()
	tx, err := db.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrapf(db.errorWrapper.WrapError(err), "failed starting transaction")
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range []struct {
		row      sqlbuild.Tuple
		conflict string
	}{
		{sqlbuild.Tuple{longTerm.UniqueID(), longTerm}, " ON CONFLICT DO NOTHING;"},
		{sqlbuild.Tuple{ephemeral.UniqueID(), longTerm}, " ON CONFLICT (ephemeral_hash) DO UPDATE SET long_term_id = EXCLUDED.long_term_id;"},
	} {
		query, args := sqlbuild.New().
			WriteString("INSERT INTO " + db.table + " (ephemeral_hash, long_term_id) VALUES ").
			WriteTuples([]sqlbuild.Tuple{stmt.row}).
			WriteString(stmt.conflict).
			Build()
		logger.Debugf("executing replace: %s", query)
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return errors.Wrapf(db.errorWrapper.WrapError(err), "failed executing query [%s]", query)
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrapf(db.errorWrapper.WrapError(err), "failed committing replace of [%s]", ephemeral)
	}
	return nil
}

// canonicalLongTerm returns the identity longTerm is bound to if that identity has a self-row,
// and longTerm otherwise. Only a self-row marks an identity as long-term, so a row that binds
// longTerm to an identity without one is not followed. PutBindings and ReplaceBinding always
// store the long term's self-row, so such a row exists only if an earlier release, which did not
// check binds, wrote it.
func (db *BindingStore) canonicalLongTerm(ctx context.Context, longTerm view.Identity) (view.Identity, error) {
	lt, err := db.GetLongTerm(ctx, longTerm)
	if err != nil {
		return nil, err
	}
	if lt.IsNone() || lt.Equal(longTerm) {
		return longTerm, nil
	}
	self, err := db.GetLongTerm(ctx, lt)
	if err != nil {
		return nil, err
	}
	if !self.Equal(lt) {
		logger.Debugf("not following binding of [%s] to [%s], which is not a long-term identity", longTerm, lt)
		return longTerm, nil
	}
	logger.Debugf("replacing [%s] with long term [%s]", longTerm, lt)
	return lt, nil
}

// splitRefused separates the ephemerals that have a self-row, and so must not be bound to
// longTerm, from the others. An ephemeral equal to longTerm is dropped from both.
func (db *BindingStore) splitRefused(ctx context.Context, longTerm view.Identity, ephemerals []view.Identity) (allowed, refused []view.Identity, err error) {
	allowed = make([]view.Identity, 0, len(ephemerals))
	for _, eph := range ephemerals {
		if eph.Equal(longTerm) {
			continue
		}
		lt, err := db.GetLongTerm(ctx, eph)
		if err != nil {
			return nil, nil, err
		}
		if lt.Equal(eph) {
			refused = append(refused, eph)
			continue
		}
		allowed = append(allowed, eph)
	}
	return allowed, refused, nil
}

func refusedError(longTerm view.Identity, refused []view.Identity) error {
	if len(refused) == 0 {
		return nil
	}
	return errors.Wrapf(driver.ErrBindingRefused, "long-term identities %v cannot be bound to [%s]", refused, longTerm)
}
