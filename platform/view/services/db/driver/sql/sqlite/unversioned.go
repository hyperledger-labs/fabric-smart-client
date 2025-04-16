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

func NewUnversionedPersistence(opts Opts) (*UnversionedPersistence, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newUnversioned(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

func NewUnversionedNotifier(opts Opts, table string) (*notifier.UnversionedPersistenceNotifier, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return notifier.NewUnversioned(newUnversioned(dbs.ReadDB, dbs.WriteDB, table)), nil
}

func newUnversioned(readDB *sql.DB, writeDB common.WriteDB, table string) *UnversionedPersistence {
	var wrapper driver.SQLErrorWrapper = &errorMapper{}
	return &UnversionedPersistence{
		UnversionedPersistence: common.NewUnversionedPersistence(writeDB, readDB, table, wrapper, NewInterpreter()),
	}
}
