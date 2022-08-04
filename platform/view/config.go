/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"

	"time"
)

// ConfigService models a configuration registry
type ConfigService struct {
	cp driver.ConfigService
}

// GetString returns the value associated with the key as a string
func (c *ConfigService) GetString(key string) string {
	return c.cp.GetString(key)
}

// GetDuration returns the value associated with the key as a duration
func (c *ConfigService) GetDuration(key string) time.Duration {
	return c.cp.GetDuration(key)
}

// GetBool returns the value associated with the key asa boolean
func (c *ConfigService) GetBool(key string) bool {
	return c.cp.GetBool(key)
}

// GetStringSlice returns the value associated with the key as a slice of strings
func (c *ConfigService) GetStringSlice(key string) []string {
	return c.cp.GetStringSlice(key)
}

// IsSet checks to see if the key has been set in any of the data locations
func (c *ConfigService) IsSet(key string) bool {
	return c.cp.IsSet(key)
}

// UnmarshalKey takes a single key and unmarshals it into a Struct
func (c *ConfigService) UnmarshalKey(key string, rawVal interface{}) error {
	return c.cp.UnmarshalKey(key, rawVal)
}

// ConfigFileUsed returns the file used to populate the config registry
func (c *ConfigService) ConfigFileUsed() string {
	return c.cp.ConfigFileUsed()
}

// GetPath allows configuration strings that specify a (config-file) relative path
func (c *ConfigService) GetPath(key string) string {
	return c.cp.GetPath(key)
}

// TranslatePath translates the passed path relative to the config path
func (c *ConfigService) TranslatePath(path string) string {
	return c.cp.TranslatePath(path)
}

// GetInt returns the value associated with the key as an integer
func (c *ConfigService) GetInt(path string) int {
	return c.cp.GetInt(path)
}

// GetConfigService returns an instance of the config service.
// It panics, if no instance is found.
func GetConfigService(sp ServiceProvider) *ConfigService {
	return &ConfigService{
		cp: driver.GetConfigService(sp),
	}
}
