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

type VersionedPersistence struct {
	*common.VersionedPersistence
}

func NewVersioned(opts common.Opts, table string) (*VersionedPersistence, error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return newVersioned(readDB, writeDB, table), nil
}

func NewVersionedNotifier(opts common.Opts, table string) (*notifier.VersionedPersistenceNotifier[*VersionedPersistence], error) {
	readDB, writeDB, err := openDB(opts.DataSource, opts.MaxOpenConns, opts.MaxIdleConns, opts.MaxIdleTime, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return notifier.NewVersioned(newVersioned(readDB, writeDB, table)), nil
}

func newVersioned(readDB, writeDB *sql.DB, table string) *VersionedPersistence {
	base := &BasePersistence[driver.VersionedValue, driver.VersionedRead]{
		BasePersistence: common.NewBasePersistence[driver.VersionedValue, driver.VersionedRead](writeDB, readDB, table, common.NewVersionedReadScanner(), common.NewVersionedValueScanner(), &errorMapper{}, NewInterpreter(), writeDB.Begin),
	}
	return &VersionedPersistence{
		VersionedPersistence: common.NewVersionedPersistence(base, table, &errorMapper{}, readDB, writeDB),
	}
}
