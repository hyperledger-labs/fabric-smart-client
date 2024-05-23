/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("db.driver.sql")

const (
	EnvVarKey = "FSC_DB_DATASOURCE"
)

type Driver struct {
}

type dbObject interface {
	CreateSchema() error
}

type persistenceConstructor[V dbObject] func(common.Opts, string) (V, error)

var versionedConstructors = map[string]persistenceConstructor[*common.VersionedPersistence]{
	"postgres": postgres.NewPersistence,
	"sqlite":   sqlite.NewVersionedPersistence,
}

var unversionedConstructors = map[string]persistenceConstructor[*common.UnversionedPersistence]{
	"postgres": postgres.NewUnversioned,
	"sqlite":   sqlite.NewUnversionedPersistence,
}

func (d *Driver) NewVersioned(dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return d.NewTransactionalVersioned(dataSourceName, config)
}

// NewTransactionalVersionedPersistence returns a new TransactionalVersionedPersistence for the passed data source and config
func (d *Driver) NewTransactionalVersioned(dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	return newPersistence[*common.VersionedPersistence](dataSourceName, config, versionedConstructors)
}

func (d *Driver) NewUnversioned(dataSourceName string, config driver.Config) (driver.UnversionedPersistence, error) {
	return newPersistence[*common.UnversionedPersistence](dataSourceName, config, unversionedConstructors)
}

func newPersistence[V dbObject](dataSourceName string, config driver.Config, constructors map[string]persistenceConstructor[V]) (V, error) {
	logger.Infof("opening new transactional database %s", dataSourceName)
	opts, err := getOps(config)
	if err != nil {
		return utils.Zero[V](), fmt.Errorf("failed getting options for datasource: %w", err)
	}

	table, valid := getTableName(opts.TablePrefix, dataSourceName)
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
		return opts, errors.New("sql driver not set in core.yaml. See ")
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

func getTableName(prefix, name string) (table string, valid bool) {
	table = fmt.Sprintf("%s_%s", prefix, name)
	r := regexp.MustCompile("^[a-zA-Z_]+$")
	return table, r.MatchString(name)
}

func init() {
	db.Register("sql", &Driver{})
}
