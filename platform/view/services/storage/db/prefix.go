/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package db

// config models the DB configuration
type config interface {
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
}

// PrefixConfig extends Config by adding a given prefix to any passed key
type PrefixConfig struct {
	config config
	prefix string
}

// NewPrefixConfig returns a ner PrefixConfig instance for the passed prefix
func NewPrefixConfig(config config, prefix string) *PrefixConfig {
	return &PrefixConfig{config: config, prefix: prefix}
}

// IsSet checks to see if the key has been set in any of the data locations
func (c *PrefixConfig) IsSet(key string) bool {
	if len(key) != 0 {
		key = c.prefix + "." + key
	} else {
		key = c.prefix
	}
	return c.config.IsSet(key)
}

// UnmarshalKey takes a single key, appends to it the prefix set in the struct, and unmarshals it into a Struct
func (c *PrefixConfig) UnmarshalKey(key string, rawVal interface{}) error {
	if len(key) != 0 {
		key = c.prefix + "." + key
	} else {
		key = c.prefix
	}
	return c.config.UnmarshalKey(key, rawVal)
}
