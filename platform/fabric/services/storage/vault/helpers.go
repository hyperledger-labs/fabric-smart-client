/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"fmt"
	"path"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	postgres3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	sqlite3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

func OpenMemoryVault(params ...string) (driver.VaultStore, error) {
	return NewStore("", multiplexed.NewDriver(&mock.ConfigProvider{}, mem.NewNamedDriver(sqlite3.NewDbProvider())), params...)
}

func OpenSqliteVault(key, tempDir string) (driver.VaultStore, error) {
	cp := sqlite3.NewConfigProvider(testing.MockConfig(sqlite3.Config{
		DataSource: fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
	}))
	return sqlite.NewPersistenceWithOpts(cp, sqlite3.NewDbProvider(), "", sqlite.NewVaultStore)
}

func OpenPostgresVault(name string) (driver.VaultStore, func(), error) {
	cfg := postgres3.DefaultConfig(postgres3.WithDBName(fmt.Sprintf("%s-db", name)))
	terminate, pgConnStr, err := postgres3.StartPostgres(context.TODO(), cfg, nil)
	if err != nil {
		return nil, nil, err
	}

	cp := postgres3.NewConfigProvider(testing.MockConfig(postgres3.Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	}))
	persistence, err := postgres2.NewPersistenceWithOpts(cp, postgres3.NewDbProvider(), "", postgres2.NewVaultStore)
	if err != nil {
		return nil, nil, err
	}
	return persistence, terminate, nil
}
