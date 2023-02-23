/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"strings"

	"github.com/pkg/errors"
)

type ConfigProvider interface {
	UnmarshalKey(key string, rawVal interface{}) error
}

type FSNConfig struct {
	Default bool   `yaml:"default,omitempty"`
	Driver  string `yaml:"driver,omitempty"`
}

type Config struct {
	configurations map[string]*FSNConfig
	names          []string
	defaultName    string
}

func NewConfig(configProvider ConfigProvider) (*Config, error) {
	var value interface{}
	if err := configProvider.UnmarshalKey("fabric", &value); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling `fabric` key")
	}
	m := value.(map[string]interface{})
	var names []string
	var defaultName string
	configurations := map[string]*FSNConfig{}
	for k := range m {
		name := k
		if strings.ToLower(name) != "enabled" {
			names = append(names, name)

			var fsnConfig FSNConfig
			if err := configProvider.UnmarshalKey("fabric."+name, &fsnConfig); err != nil {
				return nil, errors.Wrapf(err, "failed unmarshalling `fabric.%s` key", name)
			}
			configurations[name] = &fsnConfig
			logger.Debugf("found fabric network [%s], driver [%s]", name, fsnConfig.Driver)

			// is this default?
			if fsnConfig.Default {
				if len(defaultName) != 0 {
					logger.Warnf("only one network should be set as default, ignoring [%s], default is set to [%s]", name, fsnConfig.Default)
					// return nil, errors.Errorf("only one network can be set as default")
					continue
				}
				defaultName = name
			}
		}
	}
	if len(defaultName) == 0 {
		if len(names) != 0 {
			defaultName = names[0]
		} else {
			defaultName = "default"
		}
		logger.Warnf("no default network configured, set it to [%s]", defaultName)
	}

	return &Config{
		names:          names,
		defaultName:    defaultName,
		configurations: configurations,
	}, nil
}

func (c *Config) Names() []string {
	return c.names
}

func (c *Config) DefaultName() string {
	return c.defaultName
}

func (c *Config) Config(network string) (*FSNConfig, error) {
	conf, ok := c.configurations[network]
	if !ok {
		return nil, errors.Errorf("cannot find configuration for network [%s]", network)
	}
	return conf, nil
}
