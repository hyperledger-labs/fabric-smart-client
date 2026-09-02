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

// NewSignerInfoStore returns a signer-info store over table, reading through
// readDB and writing through writeDB. Its queries use $N placeholders, which
// both SQLite and Postgres accept.
func NewSignerInfoStore(writeDB WriteDB, readDB *sql.DB, table string, errorWrapper driver.SQLErrorWrapper) *SignerInfoStore {
	return &SignerInfoStore{
		table:        table,
		errorWrapper: errorWrapper,
		readDB:       readDB,
		writeDB:      writeDB,
	}
}

type SignerInfoStore struct {
	table        string
	errorWrapper driver.SQLErrorWrapper
	readDB       *sql.DB
	writeDB      WriteDB
}

func (db *SignerInfoStore) FilterExistingSigners(ctx context.Context, ids ...view.Identity) ([]view.Identity, error) {
	idHashes := make([]string, len(ids))
	inverseMap := make(map[string]view.Identity, len(ids))
	for i, id := range ids {
		idHash := id.UniqueID()
		idHashes[i] = idHash
		inverseMap[idHash] = id
	}

	query, params := sqlbuild.New().
		WriteString("SELECT id FROM " + db.table).
		WriteWhere(sqlbuild.In("id", idHashes...)).
		Build()
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
	query, params := sqlbuild.New().
		WriteString("INSERT INTO " + db.table + " (id) VALUES ").
		WriteTuples([]sqlbuild.Tuple{{id.UniqueID()}}).
		Build()

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
