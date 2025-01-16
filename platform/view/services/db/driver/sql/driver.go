/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

const (
	Postgres common.SQLDriverType = "postgres"
	SQLite   common.SQLDriverType = "sqlite"

	SQLPersistence driver2.PersistenceType = "sql"
)

var logger = logging.MustGetLogger("view-sdk.services.db.driver.sql")

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   SQLPersistence,
		Driver: &Driver{},
	}
}

type Driver struct {
}

type dbObject interface {
	CreateSchema() error
}

type persistenceConstructor[V dbObject] func(common.Opts, string) (V, error)

type transactionalVersionedPersistence interface {
	driver.TransactionalVersionedPersistence
	dbObject
}

var VersionedConstructors = map[common.SQLDriverType]persistenceConstructor[transactionalVersionedPersistence]{
	Postgres: func(o common.Opts, t string) (transactionalVersionedPersistence, error) {
		return postgres.NewVersioned(o, t)
	},
	SQLite: func(o common.Opts, t string) (transactionalVersionedPersistence, error) {
		return sqlite.NewVersioned(o, t)
	},
}

type unversionedPersistence interface {
	driver.UnversionedPersistence
	dbObject
}

var UnversionedConstructors = map[common.SQLDriverType]persistenceConstructor[unversionedPersistence]{
	Postgres: func(o common.Opts, t string) (unversionedPersistence, error) { return postgres.NewUnversioned(o, t) },
	SQLite:   func(o common.Opts, t string) (unversionedPersistence, error) { return sqlite.NewUnversioned(o, t) },
}

func (d *Driver) NewVersioned(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return d.NewTransactionalVersioned(dataSourceName, config)
}

func (d *Driver) NewTransactionalVersioned(dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	return newPersistence(dataSourceName, config, VersionedConstructors)
}

func (d *Driver) NewUnversioned(dataSourceName string, config driver.Config) (driver.UnversionedPersistence, error) {
	return newPersistence(dataSourceName, config, UnversionedConstructors)
}

func (d *Driver) NewTransactionalUnversioned(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
	backend, err := d.NewTransactionalVersioned(dataSourceName, config)
	if err != nil {
		return nil, err
	}
	return &unversioned.Transactional{TransactionalVersioned: backend}, nil
}

func newPersistence[V dbObject](dataSourceName string, config driver.Config, constructors map[common.SQLDriverType]persistenceConstructor[V]) (V, error) {
	logger.Infof("opening new transactional database %s", dataSourceName)
	opts, err := getOps(config)
	if err != nil {
		return utils.Zero[V](), fmt.Errorf("failed getting options for datasource: %w", err)
	}

	return NewPersistenceWithOpts(dataSourceName, opts, constructors)
}

func NewPersistenceWithOpts[V dbObject](dataSourceName string, opts common.Opts, constructors map[common.SQLDriverType]persistenceConstructor[V]) (V, error) {
	nc, err := common.NewTableNameCreator(opts.TablePrefix)
	if err != nil {
		return utils.Zero[V](), err
	}

	table, valid := nc.GetTableName(dataSourceName)
	if !valid {
		return utils.Zero[V](), fmt.Errorf("invalid table name [%s]: only letters and underscores allowed: %w", table, err)
	}
	c, ok := constructors[opts.Driver]
	if !ok {
		return utils.Zero[V](), fmt.Errorf("unknown driver: %s", opts.Driver)
	}
	p, err := c(opts, table)
	if err != nil {
		return utils.Zero[V](), err
	}
	if !opts.SkipCreateTable {
		if err := p.CreateSchema(); err != nil {
			return utils.Zero[V](), err
		}
	}
	return p, nil
}

func getOps(config driver.Config) (common.Opts, error) {
	opts, err := common.GetOpts(config, "")
	if err != nil {
		return common.Opts{}, err
	}
	if opts.TablePrefix == "" {
		opts.TablePrefix = "fsc"
	}
	if opts.MaxIdleTime == nil {
		opts.MaxIdleTime = common.CopyPtr(common.DefaultMaxIdleTime)
	}
	if opts.MaxIdleConns == nil {
		opts.MaxIdleConns = common.CopyPtr(common.DefaultMaxIdleConns)
	}
	return *opts, nil
}
