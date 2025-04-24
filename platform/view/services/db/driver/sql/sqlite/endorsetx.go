/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type EndorseTxStore struct {
	*common.EndorseTxStore
}

func NewEndorseTxStore(opts Opts) (*EndorseTxStore, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newEndorseTxStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.EndorseTx), nil
}

func newEndorseTxStore(readDB *sql.DB, writeDB common.WriteDB, table string) *EndorseTxStore {
	return &EndorseTxStore{EndorseTxStore: common.NewEndorseTxStore(writeDB, readDB, table, &errorMapper{}, NewInterpreter())}
}
