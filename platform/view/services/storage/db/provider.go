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
	return &configProvider{config: newPersistenceConfig(config)}
}

type configProvider struct {
	config *persistenceConfig
}

func (r *configProvider) GetConfig(persistenceName driver.PersistenceName, name string, params ...string) (driver.TableOpts, error) {
	cfg, err := r.config.Get(persistenceName)
	if err != nil {
		return nil, err
	}

	tableName, err := getTableName(cfg.Type, cfg.Opts.TablePrefix, name, params...)
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
