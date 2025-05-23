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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/query/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type VaultStore struct {
	*common.VaultStore

	tables  common.VaultTables
	writeDB common.WriteDB

	ci common2.CondInterpreter
	pi common2.PagInterpreter
}

func NewVaultStore(dbs *common3.RWDB, tables common.TableNames) (*VaultStore, error) {
	return newVaultStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), common.VaultTables{
		StateTable:  tables.State,
		StatusTable: tables.Status,
	}), nil
}

func newVaultStore(readDB *sql.DB, writeDB common.WriteDB, tables common.VaultTables) *VaultStore {
	ci := NewConditionInterpreter()
	pi := NewPaginationInterpreter()
	return &VaultStore{
		VaultStore: common.NewVaultStore(writeDB, readDB, tables, &errorMapper{}, ci, pi, newSanitizer(), isolationLevels),
		tables:     tables,
		writeDB:    writeDB,
		ci:         ci,
		pi:         pi,
	}
}

func (db *VaultStore) Store(ctx context.Context, txIDs []driver.TxID, writes driver.Writes, metaWrites driver.MetaWrites) error {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("Start store")
	defer span.AddEvent("End store")
	if len(txIDs) == 0 && len(writes) == 0 && len(metaWrites) == 0 {
		logger.Debugf("Nothing to write")
		return nil
	}

	params := make([]any, 0)
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString(`
		BEGIN;
	`)

	if len(txIDs) > 0 {
		q, ps := db.SetStatusesBusy(txIDs, len(params)+1)
		params = append(params, ps...)
		queryBuilder.WriteString(q)
		queryBuilder.WriteRune(';')
	}

	if len(writes) > 0 || len(metaWrites) > 0 {
		q, ps, err := db.UpsertStates(writes, metaWrites, len(params)+1)
		if err != nil {
			return err
		}
		params = append(params, ps...)
		queryBuilder.WriteString(q)
		queryBuilder.WriteRune(';')
	}

	if len(txIDs) > 0 {
		q, ps := db.SetStatusesValid(txIDs, len(params)+1)
		params = append(params, ps...)
		queryBuilder.WriteString(q)
		queryBuilder.WriteRune(';')
	}

	queryBuilder.WriteString(`
		COMMIT;
	`)

	query := queryBuilder.String()

	logger.Debug(query, txIDs, logging.Keys(writes))
	db.GlobalLock.RLock()
	defer db.GlobalLock.RUnlock()
	if _, err := db.writeDB.Exec(query, params...); err != nil {
		return errors.Wrapf(err, "failed to store writes and metawrites for %d txs", len(txIDs))
	}
	return nil
}

func (db *VaultStore) CreateSchema() error {
	return common.InitSchema(db.writeDB, fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		pos INTEGER PRIMARY KEY,
		tx_id TEXT UNIQUE NOT NULL,
		code INT NOT NULL DEFAULT (%d),
		message TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS tx_id_%s ON %s ( tx_id );

	CREATE TABLE IF NOT EXISTS %s (
		ns TEXT NOT NULL,
		pkey TEXT NOT NULL,
		val BYTEA DEFAULT '',
		kversion BYTEA DEFAULT '',
		metadata BYTEA DEFAULT '',
		PRIMARY KEY (pkey, ns)
	);`, db.tables.StatusTable,
		driver.Unknown,
		db.tables.StatusTable, db.tables.StatusTable,
		db.tables.StateTable))
}
