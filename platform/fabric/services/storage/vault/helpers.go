/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"fmt"
	"path"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"
)

func OpenMemoryVault(params ...string) (driver.VaultStore, error) {
	return NewStore("", multiplexed.NewDriver(&mock.ConfigProvider{}, mem.NewNamedDriver(sqlite2.NewDbProvider())), params...)
}

func OpenSqliteVault(key, tempDir string) (driver.VaultStore, error) {
	cp := sqlite2.NewConfigProvider(common.MockConfig(sqlite2.Config{
		DataSource: fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
	}))
	return sqlite.NewPersistenceWithOpts(cp, sqlite2.NewDbProvider(), "", sqlite.NewVaultStore)
}

func OpenPostgresVault(name string) (driver.VaultStore, func(), error) {
	postgresConfig := postgres.DefaultConfig(fmt.Sprintf("%s-db", name))
	terminate, err := postgres.StartPostgresWithFmt([]*postgres.ContainerConfig{postgresConfig})
	if err != nil {
		return nil, nil, err
	}

	cp := postgres.NewConfigProvider(common.MockConfig(postgres.Config{
		DataSource:   postgresConfig.DataSource(),
		MaxOpenConns: 50,
	}))
	persistence, err := postgres2.NewPersistenceWithOpts(cp, postgres.NewDbProvider(), "", postgres2.NewVaultStore)
	if err != nil {
		return nil, nil, err
	}
	return persistence, terminate, nil
}
