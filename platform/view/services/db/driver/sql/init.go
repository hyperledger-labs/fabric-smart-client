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

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("db.driver.sql")

const (
	EnvVarKey = "FSC_DB_DATASOURCE"
)

type Opts struct {
	Driver       string
	DataSource   string
	TablePrefix  string
	SchemaExists bool
	MaxOpenConns int
}

type Driver struct {
}

// NewTransactionalVersionedPersistence returns a new TransactionalVersionedPersistence for the passed data source and config
func (d *Driver) NewTransactionalVersionedPersistence(_ view.ServiceProvider, dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	logger.Infof("opening new transactional versioned database %s", dataSourceName)
	opts, err := d.getOps(config)
	if err != nil {
		return nil, fmt.Errorf("failed getting options for datasource: %w", err)
	}
	db, err := openDB(opts.Driver, opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		logger.Error(err)
		return nil, fmt.Errorf("can't open sql database: %w", err)
	}
	table, valid := getTableName(opts.TablePrefix, dataSourceName)
	if !valid {
		return nil, fmt.Errorf("invalid table name [%s]: only letters and underscores allowed: %w", table, err)
	}
	p := NewPersistence(db, table)
	if !opts.SchemaExists {
		if err := p.CreateSchema(); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (d *Driver) NewVersioned(_ view.ServiceProvider, dataSourceName string, config driver.Config) (driver.VersionedPersistence, error) {
	return d.NewTransactionalVersionedPersistence(nil, dataSourceName, config)
}

func (d *Driver) New(_ view.ServiceProvider, dataSourceName string, config driver.Config) (driver.Persistence, error) {
	logger.Infof("opening new unversioned database %s", dataSourceName)
	opts, err := d.getOps(config)
	if err != nil {
		return nil, fmt.Errorf("failed getting options for datasource")
	}
	db, err := openDB(opts.Driver, opts.DataSource, opts.MaxOpenConns)
	if err != nil {
		logger.Error(err)
		return nil, fmt.Errorf("can't open sql database: %w", err)
	}
	table, valid := getTableName(opts.TablePrefix, dataSourceName)
	if !valid {
		return nil, fmt.Errorf("invalid table name [%s]: only letters and underscores allowed", table)
	}

	p := NewUnversioned(db, table)
	if !opts.SchemaExists {
		if err := p.CreateSchema(); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func openDB(driverName, dataSourceName string, maxOpenConns int) (*sql.DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpenConns)
	if err = db.Ping(); err != nil {
		return nil, err
	}
	logger.Infof("connected to [%s,%d]", driverName, maxOpenConns)

	return db, nil
}

func (d *Driver) getOps(config driver.Config) (Opts, error) {
	opts := Opts{}
	if err := config.UnmarshalKey("", &opts); err != nil {
		return opts, fmt.Errorf("failed getting opts: %w", err)
	}
	if opts.Driver == "" {
		return opts, errors.New("sql driver not set in core.yaml. See https://github.com/golang/go/wiki/SQLDrivers")
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
