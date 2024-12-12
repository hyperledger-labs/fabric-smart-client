/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/notifier"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type UnversionedPersistence struct {
	*common.UnversionedPersistence
}

func NewUnversioned(opts common.Opts, table string) (*UnversionedPersistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newUnversioned(readDB, writeDB, table), nil
}

func NewUnversionedNotifier(opts common.Opts, table string) (*notifier.UnversionedPersistenceNotifier[*UnversionedPersistence], error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return notifier.NewUnversioned(newUnversioned(readDB, writeDB, table)), nil
}

func newUnversioned(readDB, writeDB *sql.DB, table string) *UnversionedPersistence {
	base := &BasePersistence[driver.UnversionedValue, driver.UnversionedRead]{
		BasePersistence: common.NewBasePersistence[driver.UnversionedValue, driver.UnversionedRead](writeDB, readDB, table, common.NewUnversionedReadScanner(), common.NewUnversionedValueScanner(), &errorMapper{}, NewInterpreter(), writeDB.Begin),
	}
	return &UnversionedPersistence{
		UnversionedPersistence: common.NewUnversionedPersistence(base, writeDB, table),
	}
}
