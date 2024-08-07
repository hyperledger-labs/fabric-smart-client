/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime/debug"
	"strings"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	errors2 "github.com/pkg/errors"
)

const (
	Postgres common.SQLDriverType = "postgres"
	SQLite   common.SQLDriverType = "sqlite"

	SQLPersistence driver2.PersistenceType = "sql"
)

var logger = flogging.MustGetLogger("db.driver.sql")

const (
	EnvVarKey = "FSC_DB_DATASOURCE"
)

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

var versionedConstructors = map[common.SQLDriverType]persistenceConstructor[*common.VersionedPersistence]{
	Postgres: postgres.NewPersistence,
	SQLite:   sqlite.NewVersionedPersistence,
}

var unversionedConstructors = map[common.SQLDriverType]persistenceConstructor[*common.UnversionedPersistence]{
	Postgres: postgres.NewUnversioned,
	SQLite:   sqlite.NewUnversionedPersistence,
}

func (d *Driver) NewVersioned(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return d.NewTransactionalVersioned(dataSourceName, config)
}

// NewTransactionalVersioned returns a new TransactionalVersionedPersistence for the passed data source and config
func (d *Driver) NewTransactionalVersioned(dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	return newPersistence(dataSourceName, config, versionedConstructors)
}

func (d *Driver) NewUnversioned(dataSourceName string, config driver.Config) (driver.UnversionedPersistence, error) {
	return newPersistence(dataSourceName, config, unversionedConstructors)
}

func newPersistence[V dbObject](dataSourceName string, config driver.Config, constructors map[common.SQLDriverType]persistenceConstructor[V]) (V, error) {
	logger.Infof("opening new transactional database %s", dataSourceName)
	opts, err := getOps(config)
	if err != nil {
		return utils.Zero[V](), fmt.Errorf("failed getting options for datasource: %w", err)
	}

	nc, err := NewTableNameCreator(opts.TablePrefix)
	if err != nil {
		return nil, err
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
	opts := common.Opts{}
	if err := config.UnmarshalKey("", &opts); err != nil {
		return opts, fmt.Errorf("failed getting opts: %w", err)
	}
	if opts.Driver == "" {
		return opts, errors.New("sql driver not set in core.yaml")
	}
	dataSourceOverride := os.Getenv(EnvVarKey)
	if dataSourceOverride != "" {
		logger.Infof("overriding datasource with from env var [%s] ([%d] characters)", len(dataSourceOverride), EnvVarKey)
		opts.DataSource = dataSourceOverride
	}
	if opts.DataSource == "" {
		return opts, fmt.Errorf("either the dataSource in core.yaml or %s environment variable must be set to a dataSource that can be used with the %s golang driver", EnvVarKey, opts.Driver)
	}
	if opts.TablePrefix == "" {
		opts.TablePrefix = "fsc"
	}
	return opts, nil
}

type TableNameCreator struct {
	prefix string
	r      *regexp.Regexp
}

func NewTableNameCreator(prefix string) (*TableNameCreator, error) {
	if len(prefix) > 100 {
		return nil, errors.New("table prefix must be shorter than 100 characters")
	}
	r := regexp.MustCompile("^[a-zA-Z_]+$")
	if !r.MatchString(prefix) {
		return nil, errors.New("illegal character in table prefix, only letters and underscores allowed")
	}
	return &TableNameCreator{
		prefix: strings.ToLower(prefix) + "_",
		r:      r,
	}, nil
}

func (c *TableNameCreator) GetTableName(name string) (string, bool) {
	if !c.r.MatchString(name) {
		return "", false
	}
	return fmt.Sprintf("%s%s", c.prefix, name), true
}

func (c *TableNameCreator) MustGetTableName(name string) string {
	if !c.r.MatchString(name) {
		panic("invalid name: " + name)
	}
	return fmt.Sprintf("%s%s", c.prefix, name)
}

func InitSchema(db *sql.DB, schemas ...string) (err error) {
	logger.Info("creating tables")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil && tx != nil {
			if err := tx.Rollback(); err != nil {
				logger.Errorf("failed to rollback [%s][%s]", err, debug.Stack())
			}
		}
	}()
	for _, schema := range schemas {
		logger.Info(schema)
		if _, err = tx.Exec(schema); err != nil {
			return errors2.Wrapf(err, "error creating schema: %s", schema)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return
}
