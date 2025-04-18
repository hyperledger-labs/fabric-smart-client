/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type EndorseTxPersistence struct {
	*common.EndorseTxPersistence
}

func NewEndorseTxPersistence(opts Opts) (*EndorseTxPersistence, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newEndorseTxPersistence(dbs.ReadDB, dbs.WriteDB, tables.EndorseTx), nil
}

func newEndorseTxPersistence(readDB, writeDB *sql.DB, table string) *EndorseTxPersistence {
	return &EndorseTxPersistence{EndorseTxPersistence: common.NewEndorseTxPersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
