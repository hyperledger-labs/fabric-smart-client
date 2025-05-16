/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"
	errors2 "errors"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type VaultStore struct {
	*common.VaultStore

	tables  common.VaultTables
	writeDB *sql.DB
	ci      common2.CondInterpreter
	pi      common2.PagInterpreter
}

func NewVaultStore(dbs *common3.RWDB, tables common.TableNames) (*VaultStore, error) {
	return newVaultStore(dbs.ReadDB, dbs.WriteDB, common.VaultTables{
		StateTable:  tables.State,
		StatusTable: tables.Status,
	}), nil
}

func newVaultStore(readDB, writeDB *sql.DB, tables common.VaultTables) *VaultStore {
	ci := NewConditionInterpreter()
	pi := NewPaginationInterpreter()
	return &VaultStore{
		VaultStore: common.NewVaultStore(writeDB, readDB, tables, &errorMapper{}, ci, pi, NewSanitizer(), isolationLevels),
		tables:     tables,
		writeDB:    writeDB,
		ci:         ci,
		pi:         pi,
	}
}

func (db *VaultStore) Store(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()

	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start store")
	defer span.AddEvent("End store")

	tx, err := db.writeDB.Begin()
	if err != nil {
		return errors.Wrapf(err, "failed to initiate db transaction")
	}

	if len(txIDs) > 0 {
		query, params := db.SetStatusesBusy(txIDs, 1)
		if err := execOrRollback(tx, query, params); err != nil {
			return errors.Wrapf(err, "failed setting tx to busy")
		}
	}
	if len(writes) > 0 || len(metaWrites) > 0 {
		query, params, err := db.UpsertStates(writes, metaWrites, 1)
		if err != nil {
			return err
		}
		if err := execOrRollback(tx, query, params); err != nil {
			return errors.Wrapf(err, "failed writing state")
		}
	}
	if len(txIDs) > 0 {
		query, params := db.SetStatusesValid(txIDs, 1)
		if err := execOrRollback(tx, query, params); err != nil {
			return errors.Wrapf(err, "failed setting tx to valid")
		}
	}

	if err := tx.Commit(); err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			return errors2.Join(err, err2)
		}
		return err
	}

	return nil
}

func execOrRollback(tx *sql.Tx, query string, params []any) error {
	logger.Debug(query, len(params), params)

	if _, err := tx.Exec(query, params...); err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			return errors2.Join(err, err2)
		}
		return err
	}
	return nil
}

func (db *VaultStore) CreateSchema() error {
	return common.InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		pos SERIAL PRIMARY KEY,
		tx_id TEXT UNIQUE NOT NULL,
		code INT NOT NULL DEFAULT (%d),
		message TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS tx_id_%s ON %s ( tx_id );

	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA,
		kversion BYTEA,
		metadata BYTEA,
		PRIMARY KEY (pkey, ns)
	);`, db.tables.StatusTable,
		driver.Unknown,
		db.tables.StatusTable, db.tables.StatusTable,
		db.tables.StateTable))
}
