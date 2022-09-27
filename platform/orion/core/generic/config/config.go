/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"time"

	"github.com/pkg/errors"
)

// configService models a configuration registry
type configService interface {
	// GetString returns the value associated with the key as a string
	GetString(key string) string
	// GetDuration returns the value associated with the key as a duration
	GetDuration(key string) time.Duration
	// GetBool returns the value associated with the key asa boolean
	GetBool(key string) bool
	// IsSet checks to see if the key has been set in any of the data locations
	IsSet(key string) bool
	// UnmarshalKey takes a single key and unmarshals it into a Struct
	UnmarshalKey(key string, rawVal interface{}) error
	// GetPath allows configuration strings that specify a (config-file) relative path
	GetPath(key string) string
	// TranslatePath translates the passed path relative to the config path
	TranslatePath(path string) string
	// GetInt returns the value associated with the key as an int
	GetInt(key string) int
}

type Config struct {
	name          string
	prefix        string
	configService configService
}

func New(configService configService, name string, defaultConfig bool) (*Config, error) {
	if configService.IsSet("orion." + name) {
		return &Config{
			name:          name,
			prefix:        name + ".",
			configService: configService,
		}, nil
	}

	if defaultConfig {
		return &Config{
			name:          name,
			prefix:        "",
			configService: configService,
		}, nil
	}

	return nil, errors.Errorf("configuration for [%s] not found", name)
}

// TranslatePath translates the passed path relative to the path from which the configuration has been loaded
func (c *Config) TranslatePath(path string) string {
	return c.configService.TranslatePath(path)
}

func (c *Config) GetString(key string) string {
	return c.configService.GetString("orion." + c.prefix + key)
}

func (c *Config) GetDuration(key string) time.Duration {
	return c.configService.GetDuration("orion." + c.prefix + key)
}

func (c *Config) GetBool(key string) bool {
	return c.configService.GetBool("orion." + c.prefix + key)
}

func (c *Config) IsSet(key string) bool {
	return c.configService.IsSet("orion." + c.prefix + key)
}

func (c *Config) UnmarshalKey(key string, rawVal interface{}) error {
	return c.configService.UnmarshalKey("orion."+c.prefix+key, rawVal)
}

func (c *Config) GetPath(key string) string {
	return c.configService.GetPath("orion." + c.prefix + key)
}

func (c *Config) CACert() string {
	return c.GetString("server.ca")
}

func (c *Config) ServerURL() string {
	return c.GetString("server.url")
}

func (c *Config) ServerID() string {
	return c.GetString("server.id")
}

func (c *Config) Identities() ([]Identity, error) {
	var identities []Identity
	if err := c.configService.UnmarshalKey("orion."+c.prefix+"identities", &identities); err != nil {
		return nil, err
	}
	return identities, nil
}

func (c *Config) VaultPersistenceType() string {
	return c.configService.GetString("orion." + c.prefix + "vault.persistence.type")
}

func (c *Config) VaultPersistencePrefix() string {
	return "vault.persistence.opts"
}
