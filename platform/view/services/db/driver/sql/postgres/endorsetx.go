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

func NewEndorseTxPersistence(opts common.Opts, table string) (*EndorseTxPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newEndorseTxPersistence(readWriteDB, readWriteDB, table), nil
}

func newEndorseTxPersistence(readDB, writeDB *sql.DB, table string) *EndorseTxPersistence {
	return &EndorseTxPersistence{EndorseTxPersistence: common.NewEndorseTxPersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
