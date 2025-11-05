/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common4 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

func NewKeyValueStore(dbs *common3.RWDB, tables common4.TableNames) (*common4.KeyValueStore, error) {
	return newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS), nil
}

type KeyValueStoreNotifier struct {
	*common4.KeyValueStore
	*Notifier
}

func (db *KeyValueStoreNotifier) Close() error {
	return errors.Join(
		db.KeyValueStore.Close(),
		db.Notifier.Close(),
	)
}

func (db *KeyValueStoreNotifier) CreateSchema() error {
	if err := db.KeyValueStore.CreateSchema(); err != nil {
		return err
	}
	return db.Notifier.CreateSchema()
}

func newKeyValueStore(readDB, writeDB *sql.DB, table string) *common4.KeyValueStore {
	ci := NewConditionInterpreter()
	errorWrapper := &ErrorMapper{}

	return common4.NewKeyValueStore(readDB, writeDB, table, errorWrapper, ci)
}
