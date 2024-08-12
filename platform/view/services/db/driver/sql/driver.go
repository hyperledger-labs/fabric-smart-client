/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const (
	Postgres common.SQLDriverType = "postgres"
	SQLite   common.SQLDriverType = "sqlite"

	SQLPersistence driver2.PersistenceType = "sql"

	EnvVarKey = "FSC_DB_DATASOURCE"
)

var logger = flogging.MustGetLogger("view-sdk.services.db.driver.sql")

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

var VersionedConstructors = map[common.SQLDriverType]persistenceConstructor[*common.VersionedPersistence]{
	Postgres: postgres.NewPersistence,
	SQLite:   sqlite.NewVersionedPersistence,
}

var UnversionedConstructors = map[common.SQLDriverType]persistenceConstructor[*common.UnversionedPersistence]{
	Postgres: postgres.NewUnversioned,
	SQLite:   sqlite.NewUnversionedPersistence,
}

func (d *Driver) NewVersioned(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return d.NewTransactionalVersioned(dataSourceName, config)
}

func (d *Driver) NewTransactionalVersioned(dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	return NewPersistence(dataSourceName, config, VersionedConstructors)
}

func (d *Driver) NewUnversioned(dataSourceName string, config driver.Config) (driver.UnversionedPersistence, error) {
	return NewPersistence(dataSourceName, config, UnversionedConstructors)
}

func (d *Driver) NewTransactionalUnversioned(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
	backend, err := d.NewTransactionalVersioned(dataSourceName, config)
	if err != nil {
		return nil, err
	}
	return &unversioned.Transactional{TransactionalVersioned: backend}, nil
}

func NewPersistence[V dbObject](dataSourceName string, config driver.Config, constructors map[common.SQLDriverType]persistenceConstructor[V]) (V, error) {
	logger.Infof("opening new transactional database %s", dataSourceName)
	opts, err := getOps(config)
	if err != nil {
		return utils.Zero[V](), fmt.Errorf("failed getting options for datasource: %w", err)
	}

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
	opts, err := common.GetOpts(config, "", EnvVarKey)
	if err != nil {
		return common.Opts{}, err
	}
	if opts.TablePrefix == "" {
		opts.TablePrefix = "fsc"
	}
	return *opts, nil
}
