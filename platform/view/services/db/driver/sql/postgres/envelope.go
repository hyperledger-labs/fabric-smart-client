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

type EnvelopePersistence struct {
	*common.EnvelopePersistence
}

func NewEnvelopePersistence(opts common.Opts, table string) (*EnvelopePersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newEnvelopePersistence(readWriteDB, readWriteDB, table), nil
}

func newEnvelopePersistence(readDB, writeDB *sql.DB, table string) *EnvelopePersistence {
	return &EnvelopePersistence{EnvelopePersistence: common.NewEnvelopePersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
