/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"

	"time"
)

type ConfigService struct {
	cp driver.ConfigProvider
}

func (c *ConfigService) GetString(key string) string {
	return c.cp.GetString(key)
}

func (c *ConfigService) GetDuration(key string) time.Duration {
	return c.cp.GetDuration(key)
}

func (c *ConfigService) GetBool(key string) bool {
	return c.cp.GetBool(key)
}

func (c *ConfigService) GetStringSlice(key string) []string {
	return c.cp.GetStringSlice(key)
}

func (c *ConfigService) IsSet(key string) bool {
	return c.cp.IsSet(key)
}

func (c *ConfigService) UnmarshalKey(key string, rawVal interface{}) error {
	return c.cp.UnmarshalKey(key, rawVal)
}

func (c *ConfigService) ConfigFileUsed() string {
	return c.cp.ConfigFileUsed()
}

func (c *ConfigService) GetPath(key string) string {
	return c.cp.GetPath(key)
}

func (c *ConfigService) TranslatePath(path string) string {
	return c.cp.TranslatePath(path)
}

func GetConfigService(sp ServiceProvider) *ConfigService {
	return &ConfigService{
		cp: driver.GetConfigProvider(sp),
	}
}
