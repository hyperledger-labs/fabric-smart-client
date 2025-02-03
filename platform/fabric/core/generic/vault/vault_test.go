/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	dbhelper "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/exp/slices"
)

//go:generate counterfeiter -o mocks/config.go -fake-name Config . config

type artifactsProvider struct{}

func (p *artifactsProvider) NewCachedVault(ddb dbdriver.VaultPersistence) (*vault.Vault[fdriver.ValidationCode], error) {
	return NewVault(dbhelper.NewCachedVault(ddb, 100), &disabled.Provider{}, &noop.TracerProvider{}), nil
}

func (p *artifactsProvider) NewNonCachedVault(ddb dbdriver.VaultPersistence) (*vault.Vault[fdriver.ValidationCode], error) {
	return NewVault(dbhelper.NewCachedVault(ddb, 0), &disabled.Provider{}, &noop.TracerProvider{}), nil
}

func (p *artifactsProvider) NewMarshaller() vault.Marshaller {
	return &marshaller{}
}

func TestMemory(t *testing.T) {
	vault.RemoveNils = func(items []driver2.VaultRead) []driver2.VaultRead { return items }
	ap := &artifactsProvider{}
	for _, c := range vault.SingleDBCases {
		ddb, err := dbhelper.OpenMemoryVault()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, err := dbhelper.OpenMemoryVault()
		assert.NoError(t, err)
		db2, err := dbhelper.OpenMemoryVault()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2, ap)
		})
	}
}

func TestSqlite(t *testing.T) {
	vault.RemoveNils = func(items []driver2.VaultRead) []driver2.VaultRead {
		return slices.DeleteFunc(items, func(e driver2.VaultRead) bool { return e.Raw == nil })
	}
	ap := &artifactsProvider{}
	for _, c := range vault.SingleDBCases {
		ddb, err := dbhelper.OpenSqliteVault("node1", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, err := dbhelper.OpenSqliteVault("node1", t.TempDir())
		assert.NoError(t, err)
		db2, err := dbhelper.OpenSqliteVault("node2", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2, ap)
		})
	}
}

func TestPostgres(t *testing.T) {
	vault.RemoveNils = func(items []driver2.VaultRead) []driver2.VaultRead {
		return slices.DeleteFunc(items, func(e driver2.VaultRead) bool { return e.Raw == nil })
	}
	ap := &artifactsProvider{}
	for _, c := range vault.SingleDBCases {
		ddb, terminate, err := dbhelper.OpenPostgresVault("fabric-sdk-node1")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			defer terminate()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, terminate1, err := dbhelper.OpenPostgresVault("fabric-sdk-node1")
		assert.NoError(t, err)
		db2, terminate2, err := dbhelper.OpenPostgresVault("fabric-sdk-node2")
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
