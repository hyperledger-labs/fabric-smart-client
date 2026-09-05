/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"time"
)

// ConfigService models a configuration registry
type ConfigService interface {
	// GetString returns the value associated with the key as a string
	GetString(key string) string
	// GetInt returns the value associated with the key as an integer
	GetInt(key string) int
	// GetDuration returns the value associated with the key as a duration
	GetDuration(key string) time.Duration
	// GetBool returns the value associated with the key asa boolean
	GetBool(key string) bool
	// GetStringSlice returns the value associated with the key as a slice of strings
	GetStringSlice(key string) []string
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal any) error
	// ConfigFileUsed returns the file used to populate the config registry
	ConfigFileUsed() string
	// GetPath allows configuration strings that specify a (config-file) relative path
	GetPath(key string) string
	// TranslatePath translates the passed path relative to the config path
	TranslatePath(path string) string
	// RawSubtree returns the raw map at the given key, and whether the key names a
	// subtree. Used to decode a tls: block strictly, so an unknown key is an error
	// rather than a silent discard.
	RawSubtree(key string) (map[string]any, bool)
	// RawSubtrees returns the raw maps at the given key when it holds an array of maps, as
	// a Fabric network's orderers and peers do. Array elements have no addressable key of
	// their own.
	RawSubtrees(key string) []map[string]any
}
