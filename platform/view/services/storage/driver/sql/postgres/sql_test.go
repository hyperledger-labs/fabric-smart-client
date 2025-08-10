/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	_ "modernc.org/sqlite"
)

// setupDB starts a postgres container in a go test environment.
// It registers the termination function of the container with testing.Cleanup.
func setupDB(tb testing.TB) string {
	tb.Helper()

	terminate, pgConnStr, err := StartPostgres(tb.Context(), ConfigFromEnv(), nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(terminate)

	return pgConnStr
}

func TestPostgres(t *testing.T) {
	pgConnStr := setupDB(t)
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
				Notifier:      NewNotifier(dbs.WriteDB, tables.KVS, pgConnStr, AllOperations, *NewSimplePrimaryKey("ns"), *NewBytePrimaryKey("pkey")),
			}, nil

		})
	}, func(p driver.KeyValueStore) *common3.KeyValueStore {
		return p.(*common3.KeyValueStore)
	})
}
