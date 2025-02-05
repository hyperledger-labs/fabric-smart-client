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

type EndorseTxPersistence struct {
	*common.EndorseTxPersistence
}

func NewEndorseTxPersistence(opts common.Opts, table string) (*EndorseTxPersistence, error) {
	readDB, writeDB, err := OpenRWDBs(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newEndorseTxPersistence(readDB, NewRetryWriteDB(writeDB), table), nil
}

func newEndorseTxPersistence(readDB *sql.DB, writeDB common.WriteDB, table string) *EndorseTxPersistence {
	return &EndorseTxPersistence{EndorseTxPersistence: common.NewEndorseTxPersistence(writeDB, readDB, table, &errorMapper{}, NewInterpreter())}
}
