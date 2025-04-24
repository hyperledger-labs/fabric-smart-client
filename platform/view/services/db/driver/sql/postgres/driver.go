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

func (d *Driver) NewKVS(name driver.PersistenceName, params ...string) (driver.KeyValueStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewKeyValueStore, params...)
}

func (d *Driver) NewBinding(name driver.PersistenceName, params ...string) (driver.BindingStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewBindingStore, params...)
}

func (d *Driver) NewSignerInfo(name driver.PersistenceName, params ...string) (driver.SignerInfoStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewSignerInfoStore, params...)
}

func (d *Driver) NewAuditInfo(name driver.PersistenceName, params ...string) (driver.AuditInfoStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewAuditInfoStore, params...)
}

func (d *Driver) NewEndorseTx(name driver.PersistenceName, params ...string) (driver.EndorseTxStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewEndorseTxStore, params...)
}

func (d *Driver) NewMetadata(name driver.PersistenceName, params ...string) (driver.MetadataStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewMetadataStore, params...)
}

func (d *Driver) NewEnvelope(name driver.PersistenceName, params ...string) (driver.EnvelopeStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewEnvelopeStore, params...)
}

func (d *Driver) NewVault(name driver.PersistenceName, params ...string) (driver2.VaultStore, error) {
	return NewPersistenceWithOpts(d.cp, name, NewVaultStore, params...)
}

func NewPersistenceWithOpts[V common.DBObject](cfg *configProvider, name driver.PersistenceName, constructor common.PersistenceConstructor[Opts, V], params ...string) (V, error) {
	o, err := cfg.GetOpts(name, params...)
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
