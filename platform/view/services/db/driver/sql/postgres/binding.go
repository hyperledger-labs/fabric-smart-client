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

type BindingPersistence struct {
	*common.BindingPersistence
}

func NewBindingPersistence(opts common.Opts, table string) (*BindingPersistence, error) {
	readWriteDB, err := OpenDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newBindingPersistence(readWriteDB, table), nil
}

func newBindingPersistence(readWriteDB *sql.DB, table string) *BindingPersistence {
	return &BindingPersistence{BindingPersistence: common.NewBindingPersistence(readWriteDB, readWriteDB, table, &errorMapper{}, NewInterpreter())}
}
