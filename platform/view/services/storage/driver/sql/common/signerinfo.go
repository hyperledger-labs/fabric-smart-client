/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func NewSignerInfoStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper, ph sq.PlaceholderFormat) *SignerInfoStore {
	return &SignerInfoStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
		sb:           sq.StatementBuilder.PlaceholderFormat(ph),
	}
}

type SignerInfoStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
	sb           sq.StatementBuilderType
}

func (db *SignerInfoStore) FilterExistingSigners(ctx context.Context, ids ...view.Identity) ([]view.Identity, error) {
	idHashes := make([]string, len(ids))
	inverseMap := make(map[string]view.Identity, len(ids))
	for i, id := range ids {
		idHash := id.UniqueID()
		idHashes[i] = idHash
		inverseMap[idHash] = id
	}

	inVals := make([]interface{}, len(idHashes))
	for i, h := range idHashes {
		inVals[i] = h
	}

	query, params, err := db.sb.Select("id").
		From(db.table).
		Where(sq.Eq{"id": inVals}).
		ToSql()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to build query")
	}
	logger.Debug(query, params)

	rows, err := db.readDB.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, errors.Wrapf(err, "error querying db")
	}
	defer utils.IgnoreErrorFunc(rows.Close)

	existingSigners := make([]view.Identity, 0)
	for rows.Next() {
		var idHash string
		if err := rows.Scan(&idHash); err != nil {
			return nil, errors.Wrapf(err, "failed scanning row")
		}
		existingSigners = append(existingSigners, inverseMap[idHash])
	}
	logger.DebugfContext(ctx, "Found %d out of %d signers", len(existingSigners), len(ids))
	return existingSigners, nil
}

func (db *SignerInfoStore) PutSigner(ctx context.Context, id view.Identity) error {
	query, params, err := db.sb.Insert(db.table).
		Columns("id").
		Values(id.UniqueID()).
		ToSql()
	if err != nil {
		return errors.Wrapf(err, "failed to build query")
	}

	logger.Debug(query, params)
	_, execErr := db.writeDB.ExecContext(ctx, query, params...)
	if execErr == nil {
		logger.DebugfContext(ctx, "signer [%s] registered", id)

		return nil
	}
	if errors.Is(db.errorWrapper.WrapError(execErr), driver.UniqueKeyViolation) {
		logger.InfofContext(ctx, "signer [%s] already in db. Skipping...", id)

		return nil
	}

	return errors.Wrapf(execErr, "failed executing query [%s]", query)
}

func (db *SignerInfoStore) CreateSchema() error {
	return InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT NOT NULL PRIMARY KEY
	);`, db.table))
}
