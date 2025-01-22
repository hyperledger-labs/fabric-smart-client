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

type EnvelopePersistence struct {
	*common.EnvelopePersistence
}

func NewEnvelopePersistence(opts common.Opts, table string) (*EnvelopePersistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newEnvelopePersistence(readDB, writeDB, table), nil
}

func newEnvelopePersistence(readDB, writeDB *sql.DB, table string) *EnvelopePersistence {
	return &EnvelopePersistence{EnvelopePersistence: common.NewEnvelopePersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
