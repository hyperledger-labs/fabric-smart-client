/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

// config models the DB configuration
type config interface {
	UnmarshalDriverOpts(name driver.PersistenceName, v any) error
	IsSet(key string) bool
	UnmarshalKey(key string, rawVal interface{}) error
}

type Config struct {
	TablePrefix     string
	DataSource      string
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
		o.MaxIdleConns = common3.CopyPtr(common3.DefaultMaxIdleConns)
	}
	if o.MaxIdleTime == nil {
		o.MaxIdleTime = common3.CopyPtr(common3.DefaultMaxIdleTime)
	}
	o.TableNameParams = params
	o.Tracing = &common2.TracingConfig{}

	var tlsConfig TLSConfig
	tlsKey := fmt.Sprintf("fsc.persistences.%s.opts.tls", name)
	if r.config.IsSet(tlsKey) {
		if err := r.config.UnmarshalKey(tlsKey, &tlsConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal database TLS config: %w", err)
		}
	} else {
		defaultTlsKey := "fsc.persistences.default.opts.tls"
		if r.config.IsSet(defaultTlsKey) {
			if err := r.config.UnmarshalKey(defaultTlsKey, &tlsConfig); err != nil {
				return nil, fmt.Errorf("failed to unmarshal default database TLS config: %w", err)
			}
		}
	}

	if tlsConfig.Enabled {
		registeredConnStr, err := RegisterTLSConnection(o.DataSource, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to register TLS connection config: %w", err)
		}
		o.DataSource = registeredConnStr
	}

	return o, nil
}
