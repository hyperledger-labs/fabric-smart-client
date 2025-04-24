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

type KeyValueStore struct {
	*common.KeyValueStore
}

func NewKeyValueStore(opts Opts) (*KeyValueStore, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	tables := common.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	return newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

func NewKeyValueStoreNotifier(opts Opts, table string) (*notifier.UnversionedPersistenceNotifier, error) {
	dbs, err := DbProvider.OpenDB(opts)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return notifier.NewUnversioned(newKeyValueStore(dbs.ReadDB, dbs.WriteDB, table)), nil
}

func newKeyValueStore(readDB *sql.DB, writeDB common.WriteDB, table string) *KeyValueStore {
	var wrapper driver.SQLErrorWrapper = &errorMapper{}
	return &KeyValueStore{
		KeyValueStore: common.NewKeyValueStore(writeDB, readDB, table, wrapper, NewInterpreter()),
	}
}
