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
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("db.driver.sql")

const (
	EnvVarKey     = "FSC_DB_DATASOURCE"
	sqlitePragmas = `
	PRAGMA journal_mode = WAL;
	PRAGMA busy_timeout = 5000;
	PRAGMA synchronous = NORMAL;
	PRAGMA cache_size = 1000000000;
	PRAGMA temp_store = memory;`
)

type Opts struct {
	Driver          string
	DataSource      string
	TablePrefix     string
	SkipCreateTable bool
	SkipPragmas     bool
	MaxOpenConns    int
}

type Driver struct {
}

// NewTransactionalVersionedPersistence returns a new TransactionalVersionedPersistence for the passed data source and config
func (d *Driver) NewTransactionalVersionedPersistence(_ view.ServiceProvider, dataSourceName string, config driver.Config) (driver.TransactionalVersionedPersistence, error) {
	logger.Infof("opening new transactional versioned database %s", dataSourceName)
	opts, err := getOps(config)
	if err != nil {
		return nil, fmt.Errorf("failed getting options for datasource: %w", err)
	}

	readDB, writeDB, err := openDB(opts.Driver, opts.DataSource, opts.MaxOpenConns, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	table, valid := getTableName(opts.TablePrefix, dataSourceName)
	if !valid {
		return nil, fmt.Errorf("invalid table name [%s]: only letters and underscores allowed: %w", table, err)
	}
	p := NewPersistence(readDB, writeDB, table, sqlErrorWrapper(opts.Driver))
	if !opts.SkipCreateTable {
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
	opts, err := getOps(config)
	if err != nil {
		return nil, fmt.Errorf("failed getting options for datasource")
	}

	readDB, writeDB, err := openDB(opts.Driver, opts.DataSource, opts.MaxOpenConns, opts.SkipPragmas)
	if err != nil {
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	table, valid := getTableName(opts.TablePrefix, dataSourceName)
	if !valid {
		return nil, fmt.Errorf("invalid table name [%s]: only letters and underscores allowed: %w", table, err)
	}
	p := NewUnversioned(readDB, writeDB, table, sqlErrorWrapper(opts.Driver))
	if !opts.SkipCreateTable {
		if err := p.CreateSchema(); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func sqlErrorWrapper(driver string) driver.SQLErrorWrapper {
	switch driver {
	case "postgres":
		return &postgresErrorMapper{}
	case "sqlite":
		return &sqliteErrorMapper{}
	default:
		return &noErrorMapper{}
	}
}

func openDB(driverName, dataSourceName string, maxOpenConns int, skipPragmas bool) (readDB *sql.DB, writeDB *sql.DB, err error) {
	readDB, err = sql.Open(driverName, dataSourceName)
	if err != nil {
		logger.Error(err)
		if strings.Contains(err.Error(), "out of memory (14)") {
			return nil, nil, fmt.Errorf("can't open %s database, does the folder exist?: %w", driverName, err)
		}
		return nil, nil, fmt.Errorf("can't open %s database: %w", driverName, err)
	}
	readDB.SetMaxOpenConns(maxOpenConns)
	if err = readDB.Ping(); err != nil {
		return nil, nil, err
	}
	logger.Infof("connected to [%s] for reads, max open connections: %d", driverName, maxOpenConns)

	// sqlite can handle concurrent reads in WAL mode if the writes are throttled in 1 connection
	if driverName == "sqlite" {
		writeDB, err = sql.Open(driverName, dataSourceName)
		if err != nil {
			logger.Error(err)
			return nil, nil, fmt.Errorf("can't open sql database: %w", err)
		}
		writeDB.SetMaxOpenConns(1)
		if err = writeDB.Ping(); err != nil {
			return nil, nil, err
		}
		if skipPragmas {
			if !strings.Contains(dataSourceName, "WAL") {
				logger.Warn("skipping default pragmas. Set at least ?_pragma=journal_mode(WAL) or similar in the dataSource to prevent SQLITE_BUSY errors")
			}
		} else {
			logger.Debug(sqlitePragmas)
			if _, err = readDB.Exec(sqlitePragmas); err != nil {
				return nil, nil, fmt.Errorf("error setting pragmas: %w", err)
			}
			if _, err = writeDB.Exec(sqlitePragmas); err != nil {
				return nil, nil, fmt.Errorf("error setting pragmas: %w", err)
			}
		}
		logger.Infof("connected to [%s] for writes, max open connections: 1", driverName)
	} else {
		logger.Info("using same db for writes")
		writeDB = readDB
	}
	return readDB, writeDB, nil
}

func getOps(config driver.Config) (Opts, error) {
	opts := Opts{}
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
