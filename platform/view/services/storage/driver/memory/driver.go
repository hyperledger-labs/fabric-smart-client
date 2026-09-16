/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
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

// newPersistenceWithOpts constructs a V backed by an in-memory sqlite database, using params to
// compute its table names, and invokes constructor to build the store before creating its schema.
func newPersistenceWithOpts[V common.DBObject](dbProvider sqlite2.DbProvider, constructor common2.PersistenceConstructor[V], params ...string) (V, error) {
	var zero V

	opts := Op.GetOpts(params...)
	dbs, err := dbProvider.Get(opts)
	if err != nil {
		return zero, errors.Wrap(err, "error opening db")
	}
	tables, err := common2.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	if err != nil {
		return zero, err
	}
	p, err := constructor(dbs, tables)
	if err != nil {
		return zero, err
	}
	if err := p.CreateSchema(); err != nil {
		return zero, err
	}

	return p, nil
}
