/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"fmt"
	"path"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"
)

func OpenMemoryVault(params ...string) (driver.VaultPersistence, error) {
	return NewStore(&mock.ConfigProvider{}, multiplexed.Driver{mem.NewDriver()}, params...)
}

func OpenSqliteVault(key, tempDir string) (driver.VaultPersistence, error) {
	cp := common.MockConfig(sqlite.Config{
		DataSource: fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
	})
	return sqlite.NewPersistenceWithOpts(cp, sqlite.NewVaultPersistence)
}

func OpenPostgresVault(name string) (driver.VaultPersistence, func(), error) {
	postgresConfig := postgres.DefaultConfig(fmt.Sprintf("%s-db", name))
	terminate, err := postgres.StartPostgresWithFmt([]*postgres.ContainerConfig{postgresConfig})
	if err != nil {
		return nil, nil, err
	}

	cp := common.MockConfig(postgres.Config{
		DataSource:   postgresConfig.DataSource(),
		MaxOpenConns: 50,
	})
	persistence, err := postgres.NewPersistenceWithOpts(cp, postgres.NewVaultPersistence)
	if err != nil {
		return nil, nil, err
	}
	return persistence, terminate, nil
}
