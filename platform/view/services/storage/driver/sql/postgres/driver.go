/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

const (
	Persistence cdriver.PersistenceType = "postgres"
)

func NewNamedDriver(config driver.Config, dbProvider DbProvider) driver.NamedDriver {
	return driver.NamedDriver{
		Name:   Persistence,
		Driver: NewDriverWithDbProvider(config, dbProvider),
	}
}

func NewDriver(config driver.Config) *Driver {
	return NewDriverWithDbProvider(config, NewDbProvider())
}

func NewDriverWithDbProvider(config driver.Config, dbProvider DbProvider) *Driver {
	return &Driver{
		cp:         NewConfigProvider(drivercommon.NewConfig(config)),
		dbProvider: dbProvider,
	}
}

type Driver struct {
	cp         *ConfigProvider
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

// NewPersistenceWithOpts constructs a V backed by the postgres persistence configured under name and
// params: it resolves the connection options and table names, opens the database, and invokes
// constructor to build the store, preparing its schema with migrate.CreateSchema unless configured to skip.
func NewPersistenceWithOpts[V drivercommon.DBObject](cfg *ConfigProvider, dbProvider DbProvider, name driver.PersistenceName, constructor sqlcommon.PersistenceConstructor[V], params ...string) (V, error) {
	var zero V

	o, err := cfg.GetOpts(name, params...)
	if err != nil {
		return zero, err
	}

	opts := o.Opts()
	dbs, err := dbProvider.Get(opts)
	if err != nil {
		return zero, errors.Wrap(err, "error opening db")
	}
	tables, err := sqlcommon.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	if err != nil {
		return zero, err
	}
	p, err := constructor(dbs, tables)
	if err != nil {
		return zero, err
	}
	if !o.SkipCreateTable {
		if err := migrate.CreateSchema(context.Background(), dbs.WriteDB, migrate.Postgres, opts.TablePrefix, p); err != nil {
			return zero, err
		}
	}
	return p, nil
}
