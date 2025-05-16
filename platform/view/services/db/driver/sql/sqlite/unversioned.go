/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/notifier"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
)

type KeyValueStore struct {
	*common.KeyValueStore
}

func NewKeyValueStore(dbs *common3.RWDB, tables common.TableNames) (*KeyValueStore, error) {
	return newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

func NewKeyValueStoreNotifier(dbs *common3.RWDB, table string) (*notifier.UnversionedPersistenceNotifier, error) {
	return notifier.NewUnversioned(newKeyValueStore(dbs.ReadDB, dbs.WriteDB, table)), nil
}

func newKeyValueStore(readDB *sql.DB, writeDB common.WriteDB, table string) *KeyValueStore {
	var wrapper driver.SQLErrorWrapper = &errorMapper{}
	return &KeyValueStore{
		KeyValueStore: common.NewKeyValueStore(writeDB, readDB, table, wrapper, NewConditionInterpreter()),
	}
}
