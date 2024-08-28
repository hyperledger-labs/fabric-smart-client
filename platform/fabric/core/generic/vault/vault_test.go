/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	dbhelper "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/exp/slices"
)

//go:generate counterfeiter -o mocks/config.go -fake-name Config . config

type VersionedPersistence = dbdriver.VersionedPersistence

type artifactsProvider struct{}

func (p *artifactsProvider) NewCachedVault(ddb VersionedPersistence) (*Vault, error) {
	txidStore, err := NewTXIDStore(db.Unversioned(ddb))
	if err != nil {
		return nil, err
	}
	return NewVault(ddb, txidstore.NewCache(txidStore, secondcache.NewTyped[*txidstore.Entry](100), logger), &noop.TracerProvider{}), nil
}

func (p *artifactsProvider) NewNonCachedVault(ddb VersionedPersistence) (*Vault, error) {
	txidStore, err := NewTXIDStore(db.Unversioned(ddb))
	if err != nil {
		return nil, err
	}
	return NewVault(ddb, txidstore.NewNoCache(txidStore), &noop.TracerProvider{}), nil
}

func (p *artifactsProvider) NewMarshaller() vault.Marshaller {
	return &marshaller{}
}

func TestMemory(t *testing.T) {
	vault.RemoveNils = func(items []vault.VersionedRead) []vault.VersionedRead { return items }
	ap := &artifactsProvider{}
	for _, c := range vault.SingleDBCases {
		ddb, err := dbhelper.OpenMemoryVersioned()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, err := dbhelper.OpenMemoryVersioned()
		assert.NoError(t, err)
		db2, err := dbhelper.OpenMemoryVersioned()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2, ap)
		})
	}
}

func TestBadger(t *testing.T) {
	vault.RemoveNils = func(items []vault.VersionedRead) []vault.VersionedRead { return items }
	//for _, c := range SingleDBCases {
	//	ddb, terminate, err := OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB1")
	//	assert.NoError(t, err)
	//	t.Run(c.Name, func(xt *testing.T) {
	//		defer ddb.Close()
	//		c.Fn(xt, ddb)
	//	})
	//}
	ap := &artifactsProvider{}

	for _, c := range vault.DoubleDBCases {
		db1, err := dbhelper.OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB1")
		assert.NoError(t, err)
		db2, err := dbhelper.OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2, ap)
		})
	}
}

func TestSqlite(t *testing.T) {
	vault.RemoveNils = func(items []vault.VersionedRead) []vault.VersionedRead {
		return slices.DeleteFunc(items, func(e vault.VersionedRead) bool { return e.Raw == nil })
	}
	ap := &artifactsProvider{}
	for _, c := range vault.SingleDBCases {
		ddb, err := dbhelper.OpenSqliteVersioned("node1", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, err := dbhelper.OpenSqliteVersioned("node1", t.TempDir())
		assert.NoError(t, err)
		db2, err := dbhelper.OpenSqliteVersioned("node2", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2, ap)
		})
	}
}

func TestPostgres(t *testing.T) {
	vault.RemoveNils = func(items []vault.VersionedRead) []vault.VersionedRead {
		return slices.DeleteFunc(items, func(e vault.VersionedRead) bool { return e.Raw == nil })
	}
	ap := &artifactsProvider{}
	for _, c := range vault.SingleDBCases {
		ddb, terminate, err := dbhelper.OpenPostgresVersioned("fabric-sdk-node1")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			defer terminate()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, terminate1, err := dbhelper.OpenPostgresVersioned("fabric-sdk-node1")
		assert.NoError(t, err)
		db2, terminate2, err := dbhelper.OpenPostgresVersioned("fabric-sdk-node2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2, ap)
		})
	}
}
