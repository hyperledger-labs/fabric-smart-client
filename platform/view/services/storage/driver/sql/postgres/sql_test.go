/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"os"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	_ "modernc.org/sqlite"
)

func TestPostgres(t *testing.T) {
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("set environment variable TEST_POSTGRES to true to include postgres test")
	}
	if testing.Short() {
		t.Skip("skipping postgres test in short mode")
	}
	t.Log("starting postgres")
	terminate, pgConnStr, err := StartPostgres(t, false)
	if err != nil {
		t.Fatal(err)
	}
	defer terminate()
	t.Log("postgres ready")

	cp := NewConfigProvider(testing2.MockConfig(Config{
		DataSource: pgConnStr,
	}))
	common3.TestCases(t, func(string) (driver.KeyValueStore, error) {
		return NewPersistenceWithOpts(cp, NewDbProvider(), "", NewKeyValueStore)
	}, func(string) (driver.UnversionedNotifier, error) {
		return NewPersistenceWithOpts(cp, NewDbProvider(), "", func(dbs *common.RWDB, tables common3.TableNames) (*KeyValueStoreNotifier, error) {
			return &KeyValueStoreNotifier{
				KeyValueStore: newKeyValueStore(dbs.ReadDB, dbs.WriteDB, tables.KVS),
				Notifier:      NewNotifier(dbs.WriteDB, tables.KVS, pgConnStr, AllOperations, primaryKey{"ns", identity}, primaryKey{"pkey", decode}),
			}, nil

		})
	}, func(p driver.KeyValueStore) *common3.KeyValueStore {
		return p.(*KeyValueStore).KeyValueStore
	})
}
