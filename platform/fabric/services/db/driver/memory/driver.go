/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

const Persistence driver2.PersistenceType = "memory"

type Driver struct {
	dbProvider sqlite2.DbProvider
}

func NewNamedDriver(dbProvider sqlite2.DbProvider) driver3.NamedDriver {
	return driver3.NamedDriver{
		Name:   Persistence,
		Driver: NewDriverWithDbProvider(dbProvider),
	}
}

func NewDriver() *Driver {
	return NewDriverWithDbProvider(sqlite2.NewDbProvider())
}

func NewDriverWithDbProvider(dbProvider sqlite2.DbProvider) *Driver {
	return &Driver{dbProvider: dbProvider}
}

func (d *Driver) NewEndorseTx(_ driver.PersistenceName, params ...string) (driver3.EndorseTxStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewEndorseTxStore, params...)
}

func (d *Driver) NewMetadata(_ driver.PersistenceName, params ...string) (driver3.MetadataStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewMetadataStore, params...)
}

func (d *Driver) NewEnvelope(_ driver.PersistenceName, params ...string) (driver3.EnvelopeStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewEnvelopeStore, params...)
}

func (d *Driver) NewVault(_ driver.PersistenceName, params ...string) (driver2.VaultStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewVaultStore, params...)
}

func newPersistenceWithOpts[V common.DBObject](dbProvider sqlite2.DbProvider, constructor common2.PersistenceConstructor[V], params ...string) (V, error) {
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
