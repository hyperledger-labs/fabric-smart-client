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

type BindingPersistence struct {
	*common.BindingPersistence
}

func NewBindingPersistence(opts common.Opts, table string) (*BindingPersistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newBindingPersistence(readDB, writeDB, table), nil
}

func newBindingPersistence(readDB, writeDB *sql.DB, table string) *BindingPersistence {
	return &BindingPersistence{BindingPersistence: common.NewBindingPersistence(readDB, writeDB, table, &errorMapper{}, NewInterpreter())}
}
