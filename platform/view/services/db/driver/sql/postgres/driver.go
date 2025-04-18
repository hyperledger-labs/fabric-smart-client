/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
)

const (
	Persistence driver2.PersistenceType = "postgres"
)

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   Persistence,
		Driver: &Driver{},
	}
}

type Driver struct{}

func (d *Driver) NewKVS(cfg driver.Config, params ...string) (driver.UnversionedPersistence, error) {
	return NewPersistenceWithOpts(cfg, NewUnversionedPersistence, params...)
}

func (d *Driver) NewBinding(cfg driver.Config, params ...string) (driver.BindingPersistence, error) {
	return NewPersistenceWithOpts(cfg, NewBindingPersistence, params...)
}

func (d *Driver) NewSignerInfo(cfg driver.Config, params ...string) (driver.SignerInfoPersistence, error) {
	return NewPersistenceWithOpts(cfg, NewSignerInfoPersistence, params...)
}

func (d *Driver) NewAuditInfo(cfg driver.Config, params ...string) (driver.AuditInfoPersistence, error) {
	return NewPersistenceWithOpts(cfg, NewAuditInfoPersistence, params...)
}

func (d *Driver) NewEndorseTx(cfg driver.Config, params ...string) (driver.EndorseTxPersistence, error) {
	return NewPersistenceWithOpts(cfg, NewEndorseTxPersistence, params...)
}

func (d *Driver) NewMetadata(cfg driver.Config, params ...string) (driver.MetadataPersistence, error) {
	return NewPersistenceWithOpts(cfg, NewMetadataPersistence, params...)
}

func (d *Driver) NewEnvelope(cfg driver.Config, params ...string) (driver.EnvelopePersistence, error) {
	return NewPersistenceWithOpts(cfg, NewEnvelopePersistence, params...)
}

func (d *Driver) NewVault(cfg driver.Config, params ...string) (driver2.VaultStore, error) {
	return NewPersistenceWithOpts(cfg, NewVaultPersistence, params...)
}

func NewPersistenceWithOpts[V common.DBObject](cfg driver.Config, constructor common.PersistenceConstructor[Opts, V], params ...string) (V, error) {
	o, err := NewConfigProvider(cfg).GetOpts(params...)
	if err != nil {
		return utils.Zero[V](), err
	}

	p, err := constructor(Opts{
		DataSource:      o.DataSource,
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
