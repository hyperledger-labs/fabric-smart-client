/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

const Persistence driver2.PersistenceType = "memory"

type Driver struct {
	dbProvider sqlite.DbProvider
}

func NewNamedDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   Persistence,
		Driver: NewDriver(),
	}
}

func NewDriver() *Driver {
	return &Driver{dbProvider: sqlite.NewDbProvider()}
}

func (d *Driver) NewKVS(_ driver.PersistenceName, params ...string) (driver.KeyValueStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewKeyValueStore, params...)
}

func (d *Driver) NewBinding(_ driver.PersistenceName, params ...string) (driver.BindingStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewBindingStore, params...)
}

func (d *Driver) NewSignerInfo(_ driver.PersistenceName, params ...string) (driver.SignerInfoStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewSignerInfoStore, params...)
}

func (d *Driver) NewAuditInfo(_ driver.PersistenceName, params ...string) (driver.AuditInfoStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewAuditInfoStore, params...)
}

func (d *Driver) NewEndorseTx(_ driver.PersistenceName, params ...string) (driver.EndorseTxStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewEndorseTxStore, params...)
}

func (d *Driver) NewMetadata(_ driver.PersistenceName, params ...string) (driver.MetadataStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewMetadataStore, params...)
}

func (d *Driver) NewEnvelope(_ driver.PersistenceName, params ...string) (driver.EnvelopeStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewEnvelopeStore, params...)
}

func (d *Driver) NewVault(_ driver.PersistenceName, params ...string) (driver2.VaultStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewVaultStore, params...)
}

func newPersistenceWithOpts[V common.DBObject](dbProvider sqlite.DbProvider, constructor common2.PersistenceConstructor[V], params ...string) (V, error) {
	opts := Op.GetOpts(params...)
	dbs, err := dbProvider.Get(opts)
	if err != nil {
		return utils.Zero[V](), fmt.Errorf("error opening db: %w", err)
	}
	tables := common2.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	p, err := constructor(dbs, tables)
	if err != nil {
		return utils.Zero[V](), err
	}
	if err := p.CreateSchema(); err != nil {
		return utils.Zero[V](), err
	}

	return p, nil
}
