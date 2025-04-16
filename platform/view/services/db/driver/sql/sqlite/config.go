/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
	"github.com/pkg/errors"
)

// config models the DB configuration
type config interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
}

type Config struct {
	TablePrefix     string
	DataSource      string
	SkipPragmas     bool
	MaxOpenConns    int
	MaxIdleConns    *int
	MaxIdleTime     *time.Duration
	SkipCreateTable bool
}

var tnc = db.NewTableNameCreator()

func newConfigProvider(config config) *configProvider {
	return &configProvider{config: config}
}

type configProvider struct {
	config config
}

func (r *configProvider) GetOpts() (*Config, error) {
	o := &Config{}
	if err := r.config.UnmarshalKey("opts", o); err != nil {
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
	return o, nil
}
