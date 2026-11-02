/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// config models the DB configuration: the opts of each persistence, and the
// node-wide keys that sqlcommon.MigrateOptions reads.
type config interface {
	UnmarshalDriverOpts(name driver.PersistenceName, v any) error
	sqlcommon.NodeConfig
}

type Config struct {
	TablePrefix     string
	DataSource      string
	SkipPragmas     bool
	MaxOpenConns    int
	MaxIdleConns    *int
	MaxIdleTime     *time.Duration
	SkipCreateTable bool
	TableNameParams []string
	Tracing         *sqlcommon.TracingConfig
}

func NewConfigProvider(config config) *ConfigProvider {
	return &ConfigProvider{config: config}
}

type ConfigProvider struct {
	config config
}

// MigrateOptions returns the migrate options of a persistence with the given
// skipCreateTable; see sqlcommon.MigrateOptions.
func (r *ConfigProvider) MigrateOptions(skipCreateTable bool) ([]migrate.Option, error) {
	return sqlcommon.MigrateOptions(r.config, skipCreateTable)
}

func (r *ConfigProvider) GetOpts(name driver.PersistenceName, params ...string) (*Config, error) {
	o := &Config{}
	if err := r.config.UnmarshalDriverOpts(name, o); err != nil {
		return nil, err
	}
	if len(o.DataSource) == 0 {
		return nil, errors.New("missing data source")
	}
	if o.MaxIdleConns == nil {
		o.MaxIdleConns = drivercommon.CopyPtr(drivercommon.DefaultMaxIdleConns)
	}
	if o.MaxIdleTime == nil {
		o.MaxIdleTime = drivercommon.CopyPtr(drivercommon.DefaultMaxIdleTime)
	}
	o.TableNameParams = params
	o.Tracing = &sqlcommon.TracingConfig{}
	return o, nil
}
