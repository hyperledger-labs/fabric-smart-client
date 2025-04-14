/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

const (
	Postgres common.SQLDriverType = "postgres"
	SQLite   common.SQLDriverType = "sqlite"

	SQLPersistence driver2.PersistenceType = "sql"
)

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   SQLPersistence,
		Driver: &Driver{},
	}
}

type Driver struct{}

func (d *Driver) NewKVS(tableName string, opts driver.DbOpts) (driver.UnversionedPersistence, error) {
	switch opts.Driver() {
	case Postgres:
		return common.NewPersistenceWithOpts[postgres.DbOpts](tableName, opts, postgres.NewUnversionedPersistence)
	case SQLite:
		return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewUnversionedPersistence)
	default:
		panic("unknown driver: " + opts.Driver())
	}
}

func (d *Driver) NewBinding(tableName string, opts driver.DbOpts) (driver.BindingPersistence, error) {
	switch opts.Driver() {
	case Postgres:
		return common.NewPersistenceWithOpts[postgres.DbOpts](tableName, opts, postgres.NewBindingPersistence)
	case SQLite:
		return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewBindingPersistence)
	default:
		panic("unknown driver: " + opts.Driver())
	}
}

func (d *Driver) NewSignerInfo(tableName string, opts driver.DbOpts) (driver.SignerInfoPersistence, error) {
	switch opts.Driver() {
	case Postgres:
		return common.NewPersistenceWithOpts[postgres.DbOpts](tableName, opts, postgres.NewSignerInfoPersistence)
	case SQLite:
		return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewSignerInfoPersistence)
	default:
		panic("unknown driver: " + opts.Driver())
	}
}

func (d *Driver) NewAuditInfo(tableName string, opts driver.DbOpts) (driver.AuditInfoPersistence, error) {
	switch opts.Driver() {
	case Postgres:
		return common.NewPersistenceWithOpts[postgres.DbOpts](tableName, opts, postgres.NewAuditInfoPersistence)
	case SQLite:
		return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewAuditInfoPersistence)
	default:
		panic("unknown driver: " + opts.Driver())
	}
}

func (d *Driver) NewEndorseTx(tableName string, opts driver.DbOpts) (driver.EndorseTxPersistence, error) {
	switch opts.Driver() {
	case Postgres:
		return common.NewPersistenceWithOpts[postgres.DbOpts](tableName, opts, postgres.NewEndorseTxPersistence)
	case SQLite:
		return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewEndorseTxPersistence)
	default:
		panic("unknown driver: " + opts.Driver())
	}
}

func (d *Driver) NewMetadata(tableName string, opts driver.DbOpts) (driver.MetadataPersistence, error) {
	switch opts.Driver() {
	case Postgres:
		return common.NewPersistenceWithOpts[postgres.DbOpts](tableName, opts, postgres.NewMetadataPersistence)
	case SQLite:
		return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewMetadataPersistence)
	default:
		panic("unknown driver: " + opts.Driver())
	}
}

func (d *Driver) NewEnvelope(tableName string, opts driver.DbOpts) (driver.EnvelopePersistence, error) {
	switch opts.Driver() {
	case Postgres:
		return common.NewPersistenceWithOpts[postgres.DbOpts](tableName, opts, postgres.NewEnvelopePersistence)
	case SQLite:
		return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewEnvelopePersistence)
	default:
		panic("unknown driver: " + opts.Driver())
	}
}

func (d *Driver) NewVault(tableName string, opts driver.DbOpts) (driver.VaultPersistence, error) {
	switch opts.Driver() {
	case Postgres:
		return common.NewPersistenceWithOpts[postgres.DbOpts](tableName, opts, postgres.NewVaultPersistence)
	case SQLite:
		return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewVaultPersistence)
	default:
		panic("unknown driver: " + opts.Driver())
	}
}
