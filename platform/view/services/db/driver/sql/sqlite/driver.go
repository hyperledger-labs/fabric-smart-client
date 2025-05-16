/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
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
	return &Driver{
		cp:         NewConfigProvider(common.NewConfig(config)),
		dbProvider: NewDbProvider(),
	}
}

type Driver struct {
	cp         *configProvider
	dbProvider DbProvider
}

func (d *Driver) NewKVS(name driver.PersistenceName, params ...string) (driver.KeyValueStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewKeyValueStore, params...)
}

func (d *Driver) NewBinding(name driver.PersistenceName, params ...string) (driver.BindingStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewBindingStore, params...)
}

func (d *Driver) NewSignerInfo(name driver.PersistenceName, params ...string) (driver.SignerInfoStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewSignerInfoStore, params...)
}

func (d *Driver) NewAuditInfo(name driver.PersistenceName, params ...string) (driver.AuditInfoStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewAuditInfoStore, params...)
}

func (d *Driver) NewEndorseTx(name driver.PersistenceName, params ...string) (driver.EndorseTxStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewEndorseTxStore, params...)
}

func (d *Driver) NewMetadata(name driver.PersistenceName, params ...string) (driver.MetadataStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewMetadataStore, params...)
}

func (d *Driver) NewEnvelope(name driver.PersistenceName, params ...string) (driver.EnvelopeStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewEnvelopeStore, params...)
}

func (d *Driver) NewVault(name driver.PersistenceName, params ...string) (driver2.VaultStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewVaultStore, params...)
}

func NewPersistenceWithOpts[V common.DBObject](cfg *configProvider, dbProvider DbProvider, name driver.PersistenceName, constructor common2.PersistenceConstructor[V], params ...string) (V, error) {
	o, err := cfg.GetOpts(name, params...)
	if err != nil {
		return utils.Zero[V](), err
	}

	opts := Opts{
		DataSource:      o.DataSource,
		SkipPragmas:     o.SkipPragmas,
		MaxOpenConns:    o.MaxOpenConns,
		MaxIdleConns:    *o.MaxIdleConns,
		MaxIdleTime:     *o.MaxIdleTime,
		TablePrefix:     o.TablePrefix,
		TableNameParams: o.TableNameParams,
		Tracing:         o.Tracing,
	}
	dbs, err := dbProvider.Get(opts)
	if err != nil {
		return utils.Zero[V](), fmt.Errorf("error opening db: %w", err)
	}
	tables := common2.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	p, err := constructor(dbs, tables)
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
