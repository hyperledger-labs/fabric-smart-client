/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"fmt"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

const (
	DefaultMaxIdleConns                         = 2
	DefaultMaxIdleTime                          = time.Minute
	DefaultPersistence  driver2.PersistenceName = "default"
)

//go:generate counterfeiter -o mock/config_provider.go -fake-name ConfigProvider . ConfigProvider

type ConfigProvider interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes the value corresponding to the passed key and unmarshals it into the passed structure
	UnmarshalKey(key string, rawVal interface{}) error
}

func GetPersistenceName(cs ConfigProvider, prefix string) driver2.PersistenceName {
	var persistence driver2.PersistenceName
	utils.Must(cs.UnmarshalKey(prefix, &persistence))
	return persistence
}

type Config struct {
	configProvider ConfigProvider
}

func NewConfig(configProvider ConfigProvider) *Config {
	return &Config{configProvider: configProvider}
}

func (c *Config) GetDriverType(name driver2.PersistenceName) (driver.PersistenceType, error) {
	var d driver.PersistenceType
	err := c.unmarshalPersistenceKey(name, "type", &d)
	return d, err
}

func (c *Config) UnmarshalDriverOpts(name driver2.PersistenceName, v any) error {
	return c.unmarshalPersistenceKey(name, "opts", v)
}

func (c *Config) unmarshalPersistenceKey(name driver2.PersistenceName, key string, v any) error {
	if len(name) == 0 {
		name = DefaultPersistence
	}
	return c.configProvider.UnmarshalKey(fmt.Sprintf("fsc.persistences.%s.%s", name, key), v)
}
