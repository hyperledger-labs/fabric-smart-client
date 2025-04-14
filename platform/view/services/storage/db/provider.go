/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
)

var logger = logging.MustGetLogger("db-config")

var supportedStores = collections.NewSet(mem.MemoryPersistence, sql.SQLPersistence)

// config models the DB configuration
type config interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
}

func NewConfigProvider(config config) *configProvider {
	return &configProvider{
		config:           config,
		tableNameCreator: newTableNameCreator(),
	}
}

type configProvider struct {
	config           config
	tableNameCreator *tableNameCreator
}

func (r *configProvider) GetConfig(configKey string, name string, params ...string) (driver.TableOpts, error) {
	cfg, err := r.getConfig(configKey)
	if err != nil {
		return nil, err
	}

	tableName, err := r.tableNameCreator.createTableName(cfg.Type, cfg.Opts.TablePrefix, name, params...)
	if err != nil {
		return nil, err
	}

	return &tableOpts{
		tableName:  tableName,
		driverType: cfg.Type,
		dbOpts: &opts{
			driver:          cfg.Opts.Driver,
			dataSource:      cfg.Opts.DataSource,
			skipCreateTable: cfg.Opts.SkipCreateTable,
			skipPragmas:     cfg.Opts.SkipPragmas,
			maxOpenConns:    cfg.Opts.MaxOpenConns,
			maxIdleConns:    *cfg.Opts.MaxIdleConns,
			maxIdleTime:     *cfg.Opts.MaxIdleTime,
		},
	}, nil
}

func (r *configProvider) getConfig(configKey string) (*Config, error) {
	var cfg Config
	if err := r.config.UnmarshalKey(configKey, &cfg); err != nil || !supportedStores.Contains(cfg.Type) || cfg.Type == mem.MemoryPersistence {
		logger.Warnf("Persistence type [%s]. Supported: [%v]. Error: %v", cfg.Type, supportedStores, err)
		return &Config{
			Type: mem.MemoryPersistence,
			Opts: memOpts,
		}, nil
	}

	if len(cfg.Opts.Driver) == 0 {
		return nil, notSetError("driver")
	}
	if len(cfg.Opts.DataSource) == 0 {
		return nil, notSetError("dataSource")
	}
	if cfg.Opts.MaxIdleConns == nil {
		cfg.Opts.MaxIdleConns = defaultMaxIdleConns
	}
	if cfg.Opts.MaxIdleTime == nil {
		cfg.Opts.MaxIdleTime = defaultMaxIdleTime
	}
	return &cfg, nil
}
