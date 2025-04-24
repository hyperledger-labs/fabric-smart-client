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

type SignerInfoStore struct {
	*common.SignerInfoStore
}

func NewSignerInfoStore(opts Opts) (*SignerInfoStore, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newSignerInfoStore(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.SignerInfo), nil
}

func newSignerInfoStore(readDB *sql.DB, writeDB common.WriteDB, table string) *SignerInfoStore {
	return &SignerInfoStore{SignerInfoStore: common.NewSignerInfoStore(writeDB, readDB, table, &errorMapper{}, NewInterpreter())}
}
