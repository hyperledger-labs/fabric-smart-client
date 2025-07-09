/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/notifier"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

type KeyValueStore struct {
	*common2.KeyValueStore
}

func NewKeyValueStore(dbs *common3.RWDB, tables common2.TableNames) (*KeyValueStore, error) {
	return newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

func NewKeyValueStoreNotifier(dbs *common3.RWDB, table string) (*notifier.UnversionedPersistenceNotifier, error) {
	return notifier.NewUnversioned(newKeyValueStore(dbs.ReadDB, dbs.WriteDB, table)), nil
}

func newKeyValueStore(readDB *sql.DB, writeDB common2.WriteDB, table string) *KeyValueStore {
	var wrapper driver.SQLErrorWrapper = &ErrorMapper{}
	return &KeyValueStore{
		KeyValueStore: common2.NewKeyValueStore(writeDB, readDB, table, wrapper, NewConditionInterpreter()),
	}
}
