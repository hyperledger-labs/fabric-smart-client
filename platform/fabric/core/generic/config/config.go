/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
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
}

type Config struct {
	name          string
	prefix        string
	configService configService
}

func New(configService configService, name string, defaultConfig bool) (*Config, error) {
	if configService.IsSet("fabric." + name) {
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

func (c *Config) TLSEnabled() bool {
	return c.configService.GetBool("fabric." + c.prefix + "tls.enabled")
}

func (c *Config) TLSClientAuthRequired() bool {
	return c.configService.GetBool("fabric." + c.prefix + "tls.clientAuthRequired")
}

func (c *Config) TLSServerHostOverride() string {
	return c.configService.GetString("fabric." + c.prefix + "tls.serverhostoverride")
}

func (c *Config) ClientConnTimeout() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "client.connTimeout")
}

func (c *Config) TLSClientKeyFile() string {
	return c.configService.GetPath("fabric." + c.prefix + "tls.clientKey.file")
}

func (c *Config) TLSClientCertFile() string {
	return c.configService.GetPath("fabric." + c.prefix + "tls.clientCert.file")
}

func (c *Config) TLSRootCertFile() string {
	return c.configService.GetString("fabric." + c.prefix + "tls.rootCertFile")
}

func (c *Config) KeepAliveClientInterval() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "tls.keepalive.client.interval")
}

func (c *Config) KeepAliveClientTimeout() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "tls.keepalive.client.timeout")
}

func (c *Config) KeepAliveServerInterval() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "tls.keepalive.server.interval")
}

func (c *Config) KeepAliveServerTimeout() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "tls.keepalive.server.timeout")
}

func (c *Config) KeepAliveServerMinInterval() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "tls.keepalive.server.minInterval")
}

func (c *Config) Orderers() ([]*grpc.ConnectionConfig, error) {
	var res []*grpc.ConnectionConfig
	if err := c.configService.UnmarshalKey("fabric."+c.prefix+"orderers", &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Config) Peers() ([]*grpc.ConnectionConfig, error) {
	var res []*grpc.ConnectionConfig
	if err := c.configService.UnmarshalKey("fabric."+c.prefix+"peers", &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Config) Channels() ([]*Channel, error) {
	var res []*Channel
	if err := c.configService.UnmarshalKey("fabric."+c.prefix+"channels", &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Config) VaultPersistenceType() string {
	return c.configService.GetString("fabric." + c.prefix + "vault.persistence.type")
}

func (c *Config) VaultPersistenceOpts(opts interface{}) error {
	return c.configService.UnmarshalKey("fabric."+c.prefix+"vault.persistence.opts", opts)
}

func (c *Config) MSPConfigPath() string {
	return c.configService.GetPath("fabric." + c.prefix + "mspConfigPath")
}

func (c *Config) MSPs() ([]MSP, error) {
	var confs []MSP
	if err := c.configService.UnmarshalKey("fabric."+c.prefix+"msps", &confs); err != nil {
		return nil, err
	}
	return confs, nil
}

// LocalMSPID returns the local MSP ID
func (c *Config) LocalMSPID() string {
	return c.configService.GetString("fabric." + c.prefix + "localMspId")
}

// LocalMSPType returns the local MSP Type
func (c *Config) LocalMSPType() string {
	return c.configService.GetString("fabric." + c.prefix + "localMspType")
}

// TranslatePath translates the passed path relative to the path from which the configuration has been loaded
func (c *Config) TranslatePath(path string) string {
	return c.configService.TranslatePath(path)
}

func (c *Config) Resolvers() ([]Resolver, error) {
	var resolvers []Resolver
	if err := c.configService.UnmarshalKey("fabric."+c.prefix+"endpoint.resolvers", &resolvers); err != nil {
		return nil, err
	}
	return resolvers, nil
}

func (c *Config) GetString(key string) string {
	return c.configService.GetString("fabric." + c.prefix + key)
}

func (c *Config) GetDuration(key string) time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + key)
}

func (c *Config) GetBool(key string) bool {
	return c.configService.GetBool("fabric." + c.prefix + key)
}

func (c *Config) IsSet(key string) bool {
	return c.configService.IsSet("fabric." + c.prefix + key)
}

func (c *Config) UnmarshalKey(key string, rawVal interface{}) error {
	return c.configService.UnmarshalKey("fabric."+c.prefix+key, rawVal)
}

func (c *Config) GetPath(key string) string {
	return c.configService.GetPath("fabric." + c.prefix + key)
}

func (c *Config) MSPCacheSize(defaultValue int) int {
	v := c.configService.GetString("fabric." + c.prefix + "mspCacheSize")
	if len(v) == 0 {
		return defaultValue
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return i
}
