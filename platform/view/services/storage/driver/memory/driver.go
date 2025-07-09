/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

const Persistence driver2.PersistenceType = "memory"

type Driver struct {
	dbProvider sqlite2.DbProvider
}

func NewNamedDriver(dbProvider sqlite2.DbProvider) driver.NamedDriver {
	return driver.NamedDriver{
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

func (d *Driver) NewKVS(_ driver.PersistenceName, params ...string) (driver.KeyValueStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite2.NewKeyValueStore, params...)
}

func (d *Driver) NewBinding(_ driver.PersistenceName, params ...string) (driver.BindingStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite2.NewBindingStore, params...)
}

func (d *Driver) NewSignerInfo(_ driver.PersistenceName, params ...string) (driver.SignerInfoStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite2.NewSignerInfoStore, params...)
}

func (d *Driver) NewAuditInfo(_ driver.PersistenceName, params ...string) (driver.AuditInfoStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite2.NewAuditInfoStore, params...)
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
