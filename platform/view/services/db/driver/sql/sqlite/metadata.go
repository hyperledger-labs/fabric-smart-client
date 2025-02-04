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

func NewMetadataPersistence(opts common.Opts, table string) (*MetadataPersistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newMetadataPersistence(readDB, newRetryWriteDB(writeDB), table), nil
}

func newMetadataPersistence(readDB *sql.DB, writeDB common.WriteDB, table string) *MetadataPersistence {
	return &MetadataPersistence{MetadataPersistence: common.NewMetadataPersistence(writeDB, readDB, table, &errorMapper{}, NewInterpreter())}
}
