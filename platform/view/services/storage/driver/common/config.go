/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
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
	UnmarshalKey(key string, rawVal any) error
}

// GetPersistenceName returns the persistence name configured under prefix. It returns an error
// if the configured value cannot be unmarshalled into a driver2.PersistenceName.
func GetPersistenceName(cs ConfigProvider, prefix string) (driver2.PersistenceName, error) {
	var persistence driver2.PersistenceName
	if err := cs.UnmarshalKey(prefix, &persistence); err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal persistence name for key [%s]", prefix)
	}
	return persistence, nil
}

// Config resolves the persistence configurations defined under fsc.persistences.
type Config struct {
	configProvider ConfigProvider
}

func NewConfig(configProvider ConfigProvider) *Config {
	return &Config{configProvider: configProvider}
}

// GetDriverType returns the type configured under fsc.persistences.<name>.type. An empty name
// refers to the default persistence. It returns an empty type and no error only when name is
// empty and no default persistence is defined, leaving the fallback to the caller. It returns
// an error when a non-empty name has no block under fsc.persistences, or when the referenced
// block does not set a type, so that a misspelled name or key is reported instead of being
// mistaken for an unconfigured store.
func (c *Config) GetDriverType(name driver2.PersistenceName) (driver.PersistenceType, error) {
	key := persistenceKey(name)
	if !c.configProvider.IsSet(key) {
		if len(name) == 0 {
			return "", nil
		}
		return "", errors.Errorf("persistence [%s] is not defined under fsc.persistences", name)
	}
	var d driver.PersistenceType
	if err := c.configProvider.UnmarshalKey(key+".type", &d); err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal %s.type", key)
	}
	if len(d) == 0 {
		return "", errors.Errorf("%s.type is not set", key)
	}
	return d, nil
}

func (c *Config) UnmarshalDriverOpts(name driver2.PersistenceName, v any) error {
	return c.configProvider.UnmarshalKey(persistenceKey(name)+".opts", v)
}

// persistenceKey returns the configuration key of the named persistence block, mapping an
// empty name to DefaultPersistence.
func persistenceKey(name driver2.PersistenceName) string {
	if len(name) == 0 {
		name = DefaultPersistence
	}
	return "fsc.persistences." + string(name)
}
