/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"fmt"
	"path"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/pkg/errors"
)

func OpenMemoryVault() (driver.VaultPersistence, error) {
	return (&mem.Driver{}).NewVault("", nil)
}

func OpenSqliteVault(key, tempDir string) (driver.VaultPersistence, error) {
	return (&sql.Driver{}).NewVault("test_table", &dbConfig{
		Driver:       sql.SQLite,
		DataSource:   fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
		MaxOpenConns: 0,
		SkipPragmas:  false,
	})
}

func OpenPostgresVault(name string) (driver.VaultPersistence, func(), error) {
	postgresConfig := postgres.DefaultConfig(fmt.Sprintf("%s-db", name))
	conf := &dbConfig{
		Driver:       sql.Postgres,
		DataSource:   postgresConfig.DataSource(),
		MaxOpenConns: 50,
		SkipPragmas:  false,
	}
	terminate, err := postgres.StartPostgresWithFmt([]*postgres.ContainerConfig{postgresConfig})
	if err != nil {
		return nil, nil, err
	}
	persistence, err := (&sql.Driver{}).NewVault("test_table", conf)
	return persistence, terminate, err
}

type dbConfig common.Opts

func (c *dbConfig) IsSet(string) bool { return false }
func (c *dbConfig) UnmarshalKey(key string, rawVal interface{}) error {
	if len(key) > 0 {
		return errors.New("invalid key")
	}
	if val, ok := rawVal.(*common.Opts); ok {
		*val = common.Opts(*c)
		return nil
	}
	return errors.New("invalid pointer type")
}
