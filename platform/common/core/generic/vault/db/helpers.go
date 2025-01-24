/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/pkg/errors"
)

type opener[V any] func(driver driver.Driver, dataSourceName string, config db.Config) (V, error)

func OpenMemoryVersioned() (*db.VersionedPersistence, error) {
	return openMemory[*db.VersionedPersistence](db.OpenVersioned)
}
func OpenMemory() (*db.UnversionedPersistence, error) {
	return openMemory[*db.UnversionedPersistence](db.Open)
}

func openMemory[V any](open opener[V]) (V, error) {
	return open(&mem.Driver{}, string(mem.MemoryPersistence), nil)
}

func OpenBadgerVersioned(tempDir, dir string) (*db.VersionedPersistence, error) {
	return openBadger[*db.VersionedPersistence](tempDir, dir, db.OpenVersioned)
}
func OpenBadger(tempDir, dir string) (*db.UnversionedPersistence, error) {
	return openBadger[*db.UnversionedPersistence](tempDir, dir, db.Open)
}

func openBadger[V any](tempDir, dir string, open opener[V]) (V, error) {
	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	c.IsSetReturns(false)
	return open(&badger.Driver{}, filepath.Join(tempDir, dir), c)
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

func OpenSqliteVersioned(key, tempDir string) (*db.VersionedPersistence, error) {
	return openSqlite[*db.VersionedPersistence](key, tempDir, db.OpenVersioned)
}
func OpenSqlite(key, tempDir string) (*db.UnversionedPersistence, error) {
	return openSqlite[*db.UnversionedPersistence](key, tempDir, db.Open)
}

func openSqlite[V any](key, tempDir string, open opener[V]) (V, error) {
	conf := &dbConfig{
		Driver:       sql.SQLite,
		DataSource:   fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
		MaxOpenConns: 0,
		SkipPragmas:  false,
	}
	return open(&sql.Driver{}, "test_table", conf)
}

func OpenPostgresVersioned(name string) (*db.VersionedPersistence, func(), error) {
	return openPostgres[*db.VersionedPersistence](name, db.OpenVersioned)
}
func OpenPostgres(name string) (*db.UnversionedPersistence, func(), error) {
	return openPostgres[*db.UnversionedPersistence](name, db.Open)
}

func openPostgres[V any](name string, open opener[V]) (V, func(), error) {
	postgresConfig := postgres.DefaultConfig(fmt.Sprintf("%s-db", name))
	conf := &dbConfig{
		Driver:       sql.Postgres,
		DataSource:   postgresConfig.DataSource(),
		MaxOpenConns: 50,
		SkipPragmas:  false,
	}
	terminate, err := postgres.StartPostgresWithFmt([]*postgres.ContainerConfig{postgresConfig})
	if err != nil {
		return utils.Zero[V](), func() {}, err
	}
	persistence, err := open(&sql.Driver{}, "test_table", conf)
	return persistence, terminate, err
}
