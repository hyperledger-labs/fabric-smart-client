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
	fabricpostgres "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	viewpostgres "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	viewsqlite "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

func OpenMemoryVault(params ...string) (driver.VaultStore, error) {
	return NewStore("", multiplexed.NewDriver(&mock.ConfigProvider{}, mem.NewNamedDriver(viewsqlite.NewDbProvider())), params...)
}

func OpenSqliteVault(key, tempDir string) (driver.VaultStore, error) {
	cp := viewsqlite.NewConfigProvider(testing.MockConfig(viewsqlite.Config{
		DataSource: fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
	}))
	return sqlite.NewPersistenceWithOpts(cp, viewsqlite.NewDbProvider(), "", sqlite.NewVaultStore)
}

func OpenPostgresVault(name string) (driver.VaultStore, func(), error) {
	cfg := viewpostgres.DefaultConfig(viewpostgres.WithDBName(fmt.Sprintf("%s-db", name)))
	terminate, pgConnStr, err := viewpostgres.StartPostgres(context.TODO(), cfg, nil)
	if err != nil {
		return nil, nil, err
	}

	cp := viewpostgres.NewConfigProvider(testing.MockConfig(viewpostgres.Config{
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
	}))
	persistence, err := fabricpostgres.NewPersistenceWithOpts(cp, viewpostgres.NewDbProvider(), "", fabricpostgres.NewVaultStore)
	if err != nil {
		return nil, nil, err
	}
	return persistence, terminate, nil
}
