/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/pkg/errors"
)

// config models the DB configuration
type config interface {
	UnmarshalDriverOpts(name driver.PersistenceName, v any) error
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
	Tracing         *common2.TracingConfig
}

func NewConfigProvider(config config) *ConfigProvider {
	return &ConfigProvider{config: config}
}

type ConfigProvider struct {
	config config
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
		o.MaxIdleConns = common.CopyPtr(common.DefaultMaxIdleConns)
	}
	if o.MaxIdleTime == nil {
		o.MaxIdleTime = common.CopyPtr(common.DefaultMaxIdleTime)
	}
	o.TableNameParams = params
	o.Tracing = &common2.TracingConfig{}
	return o, nil
}
