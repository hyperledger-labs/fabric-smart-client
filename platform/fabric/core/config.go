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

type Config struct {
	names       []string
	defaultName string
}

func NewConfig(configProvider ConfigProvider) (*Config, error) {
	var value interface{}
	if err := configProvider.UnmarshalKey("fabric", &value); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling `fabric` key")
	}
	m := value.(map[interface{}]interface{})
	var names []string
	var defaultName string
	for k, v := range m {
		name := k.(string)
		if strings.ToLower(name) != "enabled" {
			names = append(names, name)
			// is this default?
			defaultValue, ok := (v.(map[interface{}]interface{}))["default"]
			if ok && defaultValue.(bool) {
				if len(defaultName) != 0 {
					return nil, errors.Errorf("only one network can be set as default")
				}
				defaultName = name
			}
		}
	}
	if len(defaultName) == 0 {
		logger.Warnf("no default network configured, set it to [default]")
		defaultName = "default"
	}

	return &Config{
		names:       names,
		defaultName: defaultName,
	}, nil
}

func (c *Config) Names() []string {
	return c.names
}

func (c *Config) DefaultName() string {
	return c.defaultName
}
