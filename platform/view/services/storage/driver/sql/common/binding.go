/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const BindingStoreMaxEphemerals = 1000

func NewBindingStore(readDB *sql.DB, writeDB WriteDB, table string, errorWrapper driver.SQLErrorWrapper, ph sq.PlaceholderFormat) *BindingStore {
	return &BindingStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		sb:           sq.StatementBuilder.PlaceholderFormat(ph),
	}
}

type BindingStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	sb           sq.StatementBuilderType // squirrel builder, internal only
}

func (db *BindingStore) GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error) {
	query, params, err := db.sb.Select("long_term_id").
		From(db.table).
		Where(sq.Eq{"ephemeral_hash": ephemeral.UniqueID()}).
		ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}

	logger.Debug(query, params)
	result, err := QueryUniqueContext[view.Identity](ctx, db.readDB, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting wallet id for identity [%v]", ephemeral)
	}
	logger.DebugfContext(ctx, "found wallet id for identity [%v]: %v", ephemeral, result)
	return result, nil
}

func (db *BindingStore) HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error) {
	query, params, err := db.sb.Select("long_term_id").
		From(db.table).
		Where(sq.Eq{"ephemeral_hash": []string{this.UniqueID(), that.UniqueID()}}).
		ToSql()
	if err != nil {
		return false, errors.Wrapf(err, "failed to build query")
	}

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

	// Build single INSERT with multiple VALUES
	// prepare query placeholder and arguments
	placeholders := make([]string, len(ephemerals)+1)
	args := make([]any, 0, (len(ephemerals)+1)*2)

	// first item it the longTerm itself
	i := 0
	placeholders[i] = fmt.Sprintf("($%d,$%d)", i*2+1, i*2+2)
	args = append(args, longTerm.UniqueID(), longTerm)

	// next we go through our ephemerals
	for _, eph := range ephemerals {
		i++
		placeholders[i] = fmt.Sprintf("($%d,$%d)", i*2+1, i*2+2)
		args = append(args, eph.UniqueID(), longTerm)
	}

	query := fmt.Sprintf(`INSERT INTO %s (ephemeral_hash, long_term_id) VALUES %s ON CONFLICT DO NOTHING;`,
		db.table, strings.Join(placeholders, ","))

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
