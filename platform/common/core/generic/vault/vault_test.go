/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
)

//go:generate counterfeiter -o mocks/config.go -fake-name Config . config

var removeNils func(items []driver.VersionedRead) []driver.VersionedRead

func TestMemory(t *testing.T) {
	removeNils = func(items []driver.VersionedRead) []driver.VersionedRead { return items }
	for _, c := range SingleDBCases {
		ddb, err := vault.OpenMemoryVersioned()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			c.Fn(xt, ddb)
		})
	}

	for _, c := range DoubleDBCases {
		db1, err := vault.OpenMemoryVersioned()
		assert.NoError(t, err)
		db2, err := vault.OpenMemoryVersioned()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2)
		})
	}
}

func TestBadger(t *testing.T) {
	removeNils = func(items []driver.VersionedRead) []driver.VersionedRead { return items }
	//for _, c := range SingleDBCases {
	//	ddb, terminate, err := vault.OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB1")
	//	assert.NoError(t, err)
	//	t.Run(c.Name, func(xt *testing.T) {
	//		defer ddb.Close()
	//		c.Fn(xt, ddb)
	//	})
	//}

	for _, c := range DoubleDBCases {
		db1, err := vault.OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB1")
		assert.NoError(t, err)
		db2, err := vault.OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2)
		})
	}
}

func TestSqlite(t *testing.T) {
	removeNils = func(items []driver.VersionedRead) []driver.VersionedRead {
		return slices.DeleteFunc(items, func(e driver.VersionedRead) bool { return e.Raw == nil })
	}

	for _, c := range SingleDBCases {
		ddb, err := vault.OpenSqliteVersioned("node1", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			c.Fn(xt, ddb)
		})
	}

	for _, c := range DoubleDBCases {
		db1, err := vault.OpenSqliteVersioned("node1", t.TempDir())
		assert.NoError(t, err)
		db2, err := vault.OpenSqliteVersioned("node2", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2)
		})
	}
}

func TestPostgres(t *testing.T) {
	removeNils = func(items []driver.VersionedRead) []driver.VersionedRead {
		return slices.DeleteFunc(items, func(e driver.VersionedRead) bool { return e.Raw == nil })
	}

	for _, c := range SingleDBCases {
		ddb, terminate, err := vault.OpenPostgresVersioned("node1")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			defer terminate()
			c.Fn(xt, ddb)
		})
	}

	for _, c := range DoubleDBCases {
		db1, terminate1, err := vault.OpenPostgresVersioned("node1")
		assert.NoError(t, err)
		db2, terminate2, err := vault.OpenPostgresVersioned("node2")
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
