/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

import "fmt"

type PrefixConfig struct {
	config Config
	prefix string
}

func NewPrefixConfig(config Config, prefix string) *PrefixConfig {
	return &PrefixConfig{config: config, prefix: prefix}
}

func (c *PrefixConfig) UnmarshalKey(key string, rawVal interface{}) error {
	fmt.Println("PrefixConfig.UnmarshalKey:", c.prefix, key, rawVal)
	if len(key) != 0 {
		key = c.prefix + "." + key
	} else {
		key = c.prefix
	}
	return c.config.UnmarshalKey(key, rawVal)
}
