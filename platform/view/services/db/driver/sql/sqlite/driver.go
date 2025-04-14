/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
)

const (
	Persistence driver2.PersistenceType = "sqlite"
)

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   Persistence,
		Driver: &Driver{},
	}
}

type Driver struct{}

func (d *Driver) NewKVS(tableName string, cfg driver.Config) (driver.UnversionedPersistence, error) {
	return NewPersistenceWithOpts(tableName, cfg, NewUnversionedPersistence)
}

func (d *Driver) NewBinding(tableName string, cfg driver.Config) (driver.BindingPersistence, error) {
	return NewPersistenceWithOpts(tableName, cfg, NewBindingPersistence)
}

func (d *Driver) NewSignerInfo(tableName string, cfg driver.Config) (driver.SignerInfoPersistence, error) {
	return NewPersistenceWithOpts(tableName, cfg, NewSignerInfoPersistence)
}

func (d *Driver) NewAuditInfo(tableName string, cfg driver.Config) (driver.AuditInfoPersistence, error) {
	return NewPersistenceWithOpts(tableName, cfg, NewAuditInfoPersistence)
}

func (d *Driver) NewEndorseTx(tableName string, cfg driver.Config) (driver.EndorseTxPersistence, error) {
	return NewPersistenceWithOpts(tableName, cfg, NewEndorseTxPersistence)
}

func (d *Driver) NewMetadata(tableName string, cfg driver.Config) (driver.MetadataPersistence, error) {
	return NewPersistenceWithOpts(tableName, cfg, NewMetadataPersistence)
}

func (d *Driver) NewEnvelope(tableName string, cfg driver.Config) (driver.EnvelopePersistence, error) {
	return NewPersistenceWithOpts(tableName, cfg, NewEnvelopePersistence)
}

func (d *Driver) NewVault(tableName string, cfg driver.Config) (driver.VaultPersistence, error) {
	return NewPersistenceWithOpts(tableName, cfg, NewVaultPersistence)
}

func NewPersistenceWithOpts[V common.DBObject](tableName string, cfg driver.Config, constructor common.PersistenceConstructor[Opts, V]) (V, error) {
	o, err := newConfigProvider(cfg).GetOpts()
	if err != nil {
		return utils.Zero[V](), err
	}
	table, err := tnc.CreateTableName(tableName, o.TablePrefix)
	if err != nil {
		return utils.Zero[V](), err
	}

	p, err := constructor(Opts{
		DataSource:   o.DataSource,
		SkipPragmas:  o.SkipPragmas,
		MaxOpenConns: o.MaxOpenConns,
		MaxIdleConns: *o.MaxIdleConns,
		MaxIdleTime:  *o.MaxIdleTime,
	}, table)
	if err != nil {
		return utils.Zero[V](), err
	}
	if !o.SkipCreateTable {
		if err := p.CreateSchema(); err != nil {
			return utils.Zero[V](), err
		}
	}
	return p, nil
}
