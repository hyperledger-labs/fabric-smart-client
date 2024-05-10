/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/pkg/errors"
)

type opener[V any] func(driverName, dataSourceName string, config db.Config) (V, error)

func OpenMemoryVersioned() (driver.VersionedPersistence, error) {
	return openMemory[driver.VersionedPersistence](db.OpenVersioned)
}
func OpenMemory() (driver.Persistence, error) {
	return openMemory[driver.Persistence](db.Open)
}

func openMemory[V any](open opener[V]) (V, error) {
	return open("memory", "", nil)
}

func OpenBadgerVersioned(tempDir, dir string) (driver.VersionedPersistence, error) {
	return openBadger[driver.VersionedPersistence](tempDir, dir, db.OpenVersioned)
}
func OpenBadger(tempDir, dir string) (driver.Persistence, error) {
	return openBadger[driver.Persistence](tempDir, dir, db.Open)
}

func openBadger[V any](tempDir, dir string, open opener[V]) (V, error) {
	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	c.IsSetReturns(false)
	return open("badger", filepath.Join(tempDir, dir), c)
}

type dbConfig common.Opts

func (c *dbConfig) IsSet(string) bool { panic("not supported") }
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

type driverOpener[V any] func(dataSourceName string, config driver.Config) (V, error)

func OpenSqliteVersioned(key, tempDir string) (driver.VersionedPersistence, error) {
	return openSqlite[driver.VersionedPersistence](key, tempDir, (&sql.Driver{}).NewVersioned)
}
func OpenSqlite(key, tempDir string) (driver.Persistence, error) {
	return openSqlite[driver.Persistence](key, tempDir, (&sql.Driver{}).New)
}

func openSqlite[V any](key, tempDir string, open driverOpener[V]) (V, error) {
	conf := &dbConfig{
		Driver:       "sqlite",
		DataSource:   fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
		MaxOpenConns: 0,
		SkipPragmas:  false,
	}
	return open("test_table", conf)
}

func OpenPostgresVersioned(name string) (driver.VersionedPersistence, func(), error) {
	return openPostgres[driver.VersionedPersistence](name, (&sql.Driver{}).NewVersioned)
}
func OpenPostgres(name string) (driver.Persistence, func(), error) {
	return openPostgres[driver.Persistence](name, (&sql.Driver{}).New)
}

func openPostgres[V any](name string, open driverOpener[V]) (V, func(), error) {
	postgresConfig := common2.DefaultConfig(fmt.Sprintf("%s-db", name))
	conf := &dbConfig{
		Driver:       "postgres",
		DataSource:   postgresConfig.DataSource(),
		MaxOpenConns: 50,
		SkipPragmas:  false,
	}
	terminate, err := common2.StartPostgresWithFmt(map[string]*common2.PostgresConfig{name: postgresConfig})
	if err != nil {
		return utils.Zero[V](), func() {}, err
	}
	persistence, err := open("test_table", conf)
	return persistence, terminate, err
}
