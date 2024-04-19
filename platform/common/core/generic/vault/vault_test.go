/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mocks/config.go -fake-name Config . config

var tempDir string

func TestMemory(t *testing.T) {
	for _, c := range SingleDBCases {
		ddb, terminate, err := openMemory()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			defer terminate()
			c.Fn(xt, ddb)
		})
	}

	for _, c := range DoubleDBCases {
		db1, terminate1, err := openMemory()
		assert.NoError(t, err)
		db2, terminate2, err := openMemory()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2)
		})
	}
}

func TestBadger(t *testing.T) {
	//for _, c := range SingleDBCases {
	//	ddb, terminate, err := openBadger("DB-TestVaultBadgerDB1")
	//	assert.NoError(t, err)
	//	t.Run(c.Name, func(xt *testing.T) {
	//		defer ddb.Close()
	//		defer terminate()
	//		c.Fn(xt, ddb)
	//	})
	//}

	for _, c := range DoubleDBCases {
		db1, terminate1, err := openBadger("DB-TestVaultBadgerDB1")
		assert.NoError(t, err)
		db2, terminate2, err := openBadger("DB-TestVaultBadgerDB2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2)
		})
	}
}

func TestSqlite(t *testing.T) {
	tempDir = t.TempDir()

	//for _, c := range SingleDBCases {
	//	ddb, terminate, err := openSqlite("node1")
	//	assert.NoError(t, err)
	//	t.Run(c.Name, func(xt *testing.T) {
	//		defer ddb.Close()
	//		defer terminate()
	//		c.Fn(xt, ddb)
	//	})
	//}

	for _, c := range DoubleDBCases {
		db1, terminate1, err := openSqlite("node1")
		assert.NoError(t, err)
		db2, terminate2, err := openSqlite("node2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2)
		})
	}
}

func TestPostgres(t *testing.T) {
	//for _, c := range SingleDBCases {
	//	ddb, terminate, err := openPostgres("node1")
	//	assert.NoError(t, err)
	//	t.Run(c.Name, func(xt *testing.T) {
	//		defer ddb.Close()
	//		defer terminate()
	//		c.Fn(xt, ddb)
	//	})
	//}

	for _, c := range DoubleDBCases {
		db1, terminate1, err := openPostgres("node1")
		assert.NoError(t, err)
		db2, terminate2, err := openPostgres("node2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2)
		})
	}
}

func TestMain(m *testing.M) {
	var err error
	tempDir, err = os.MkdirTemp("", "vault-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temporary directory: %v", err)
		os.Exit(-1)
	}
	defer os.RemoveAll(tempDir)

	m.Run()
}

// Open DB utils

func openMemory() (driver.VersionedPersistence, func(), error) {
	persistence, err := db.OpenVersioned(nil, "memory", "", nil)
	return persistence, func() {}, err
}

func openBadger(dir string) (driver.VersionedPersistence, func(), error) {
	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	c.IsSetReturns(false)
	persistence, err := db.OpenVersioned(nil, "badger", filepath.Join(tempDir, dir), c)
	return persistence, func() {}, err
}

type dbConfig sql.Opts

func (c *dbConfig) IsSet(string) bool { panic("not supported") }
func (c *dbConfig) UnmarshalKey(key string, rawVal interface{}) error {
	if len(key) > 0 {
		return errors.New("invalid key")
	}
	fmt.Printf("here opts: %v", reflect.TypeOf(rawVal))
	if val, ok := rawVal.(*sql.Opts); ok {
		*val = sql.Opts(*c)
		return nil
	}
	return errors.New("invalid pointer type")
}

func openSqlite(key string) (driver.VersionedPersistence, func(), error) {
	conf := &dbConfig{
		Driver:       "sqlite",
		DataSource:   fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)),
		MaxOpenConns: 0,
		SkipPragmas:  false,
	}
	persistence, err := (&sql.Driver{}).NewVersioned(nil, "test_table", conf)
	return persistence, func() {}, err
}

func openPostgres(name string) (driver.VersionedPersistence, func(), error) {
	postgresConfig := sql.DefaultConfig(fmt.Sprintf("%s-db", name))
	conf := &dbConfig{
		Driver:       "postgres",
		DataSource:   postgresConfig.DataSource(),
		MaxOpenConns: 50,
		SkipPragmas:  false,
	}
	terminate, err := sql.StartPostgresWithFmt(map[string]*sql.PostgresConfig{name: postgresConfig})
	if err != nil {
		return nil, func() {}, err
	}
	persistence, err := (&sql.Driver{}).NewVersioned(nil, "test_table", conf)
	return persistence, terminate, err
}
