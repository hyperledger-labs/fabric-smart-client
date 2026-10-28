/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	fvault "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

type artifactsProvider struct {
	removeNils func([]driver.VaultRead) []driver.VaultRead
}

func (p *artifactsProvider) RemoveNils(items []driver.VaultRead) []driver.VaultRead {
	return p.removeNils(items)
}

func (*artifactsProvider) NewCachedVault(ddb dbdriver.VaultStore) (*vault.Vault[fdriver.ValidationCode], error) {
	return NewVault(fvault.NewCachedVault(ddb, 100), &disabled.Provider{}, &noop.TracerProvider{}), nil
}

func (*artifactsProvider) NewNonCachedVault(ddb dbdriver.VaultStore) (*vault.Vault[fdriver.ValidationCode], error) {
	return NewVault(fvault.NewCachedVault(ddb, 0), &disabled.Provider{}, &noop.TracerProvider{}), nil
}

func (*artifactsProvider) NewMarshaller() vault.Marshaller {
	return &marshaller{}
}

func TestMemory(t *testing.T) {
	t.Parallel()
	ap := &artifactsProvider{
		removeNils: func(items []driver.VaultRead) []driver.VaultRead { return items },
	}
	for _, c := range vault.SingleDBCases {
		ddb, err := fvault.OpenMemoryVault(c.Name)
		require.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer func() { _ = ddb.Close() }()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, err := fvault.OpenMemoryVault(c.Name)
		require.NoError(t, err)
		db2, err := fvault.OpenMemoryVault(c.Name)
		require.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			xt.Parallel()
			defer func() { _ = db1.Close() }()
			defer func() { _ = db2.Close() }()
			c.Fn(xt, db1, db2, ap)
		})
	}
}

func TestSqlite(t *testing.T) {
	t.Parallel()
	ap := &artifactsProvider{
		removeNils: func(items []driver.VaultRead) []driver.VaultRead {
			return slices.DeleteFunc(items, func(e driver.VaultRead) bool { return e.Raw == nil }) //nolint:govet
		},
	}
	for _, c := range vault.SingleDBCases {
		ddb, err := fvault.OpenSqliteVault("node1", t.TempDir())
		require.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer func() { _ = ddb.Close() }()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, err := fvault.OpenSqliteVault("node1", t.TempDir())
		require.NoError(t, err)
		db2, err := fvault.OpenSqliteVault("node2", t.TempDir())
		require.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			xt.Parallel()
			defer func() { _ = db1.Close() }()
			defer func() { _ = db2.Close() }()
			c.Fn(xt, db1, db2, ap)
		})
	}
}

func TestPostgres(t *testing.T) {
	t.Parallel()
	ap := &artifactsProvider{
		removeNils: func(items []driver.VaultRead) []driver.VaultRead {
			return slices.DeleteFunc(items, func(e driver.VaultRead) bool { return e.Raw == nil }) //nolint:govet
		},
	}
	for _, c := range vault.SingleDBCases {
		ddb, terminate, err := fvault.OpenPostgresVault("fabric-sdk-node1")
		require.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer func() { _ = ddb.Close() }()
			defer terminate()
			c.Fn(xt, ddb, ap)
		})
	}

	for _, c := range vault.DoubleDBCases {
		db1, terminate1, err := fvault.OpenPostgresVault("fabric-sdk-node1")
		require.NoError(t, err)
		db2, terminate2, err := fvault.OpenPostgresVault("fabric-sdk-node2")
		require.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			xt.Parallel()
			defer func() { _ = db1.Close() }()
			defer func() { _ = db2.Close() }()
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2, ap)
		})
	}
}
