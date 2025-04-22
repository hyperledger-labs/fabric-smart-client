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

func NewNamedDriver(config driver.Config) driver.NamedDriver {
	return driver.NamedDriver{
		Name:   Persistence,
		Driver: NewDriver(config),
	}
}

func NewDriver(config driver.Config) *Driver {
	return &Driver{cp: NewConfigProvider(common.NewConfig(config))}
}

type Driver struct {
	cp *configProvider
}

func (d *Driver) NewKVS(name driver.PersistenceName, params ...string) (driver.UnversionedPersistence, error) {
	return NewPersistenceWithOpts(d.cp, name, NewUnversionedPersistence, params...)
}

func (d *Driver) NewBinding(name driver.PersistenceName, params ...string) (driver.BindingPersistence, error) {
	return NewPersistenceWithOpts(d.cp, name, NewBindingPersistence, params...)
}

func (d *Driver) NewSignerInfo(name driver.PersistenceName, params ...string) (driver.SignerInfoPersistence, error) {
	return NewPersistenceWithOpts(d.cp, name, NewSignerInfoPersistence, params...)
}

func (d *Driver) NewAuditInfo(name driver.PersistenceName, params ...string) (driver.AuditInfoPersistence, error) {
	return NewPersistenceWithOpts(d.cp, name, NewAuditInfoPersistence, params...)
}

func (d *Driver) NewEndorseTx(name driver.PersistenceName, params ...string) (driver.EndorseTxPersistence, error) {
	return NewPersistenceWithOpts(d.cp, name, NewEndorseTxPersistence, params...)
}

func (d *Driver) NewMetadata(name driver.PersistenceName, params ...string) (driver.MetadataPersistence, error) {
	return NewPersistenceWithOpts(d.cp, name, NewMetadataPersistence, params...)
}

func (d *Driver) NewEnvelope(name driver.PersistenceName, params ...string) (driver.EnvelopePersistence, error) {
	return NewPersistenceWithOpts(d.cp, name, NewEnvelopePersistence, params...)
}

func (d *Driver) NewVault(name driver.PersistenceName, params ...string) (driver2.VaultStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewVaultPersistence, params...)
}

func NewPersistenceWithOpts[V common.DBObject](cfg *configProvider, name driver.PersistenceName, constructor common.PersistenceConstructor[Opts, V], params ...string) (V, error) {
	o, err := cfg.GetOpts(name, params...)
	if err != nil {
		return utils.Zero[V](), err
	}

	p, err := constructor(Opts{
		DataSource:      o.DataSource,
		SkipPragmas:     o.SkipPragmas,
		MaxOpenConns:    o.MaxOpenConns,
		MaxIdleConns:    *o.MaxIdleConns,
		MaxIdleTime:     *o.MaxIdleTime,
		TablePrefix:     o.TablePrefix,
		TableNameParams: o.TableNameParams,
	})
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
