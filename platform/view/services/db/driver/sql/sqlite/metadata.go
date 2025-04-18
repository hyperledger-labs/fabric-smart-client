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

type MetadataPersistence struct {
	*common.MetadataPersistence
}

func NewMetadataPersistence(opts Opts) (*MetadataPersistence, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newMetadataPersistence(dbs.ReadDB, NewRetryWriteDB(dbs.WriteDB), tables.Metadata), nil
}

func newMetadataPersistence(readDB *sql.DB, writeDB common.WriteDB, table string) *MetadataPersistence {
	return &MetadataPersistence{MetadataPersistence: common.NewMetadataPersistence(writeDB, readDB, table, &errorMapper{}, NewInterpreter())}
}
