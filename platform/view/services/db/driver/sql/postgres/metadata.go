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

func NewMetadataPersistence(opts common.Opts, table string) (*MetadataPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newMetadataPersistence(readWriteDB, readWriteDB, table), nil
}

func newMetadataPersistence(readDB, writeDB *sql.DB, table string) *MetadataPersistence {
	return &MetadataPersistence{MetadataPersistence: common.NewMetadataPersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
