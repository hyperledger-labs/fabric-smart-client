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

type MetadataPersistence struct {
	*common.MetadataPersistence
}

func NewMetadataPersistence(opts Opts) (*MetadataPersistence, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newMetadataPersistence(dbs.ReadDB, dbs.WriteDB, tables.Metadata), nil
}

func newMetadataPersistence(readDB, writeDB *sql.DB, table string) *MetadataPersistence {
	return &MetadataPersistence{MetadataPersistence: common.NewMetadataPersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
