/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/pkg/errors"
)

const (
	DefaultMSPCacheSize               = 3
	DefaultBroadcastNumRetries        = 3
	VaultPersistenceOptsKey           = "vault.persistence.opts"
	DefaultOrderingConnectionPoolSize = 10
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.config")

// configService models a configuration registry
type configService interface {
	// GetString returns the value associated with the key as a string
	GetString(key string) string
	// GetDuration returns the value associated with the key as a duration
	GetDuration(key string) time.Duration
	// GetBool returns the value associated with the key as a boolean
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

func (c *Config) Name() string {
	return c.name
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

func (c *Config) KeepAliveClientInterval() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "keepalive.interval")
}

func (c *Config) KeepAliveClientTimeout() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "keepalive.timeout")
}

func (c *Config) Orderers() ([]*grpc.ConnectionConfig, error) {
	var res []*grpc.ConnectionConfig
	if err := c.configService.UnmarshalKey("fabric."+c.prefix+"orderers", &res); err != nil {
		return nil, err
	}

	for _, v := range res {
		v.TLSEnabled = c.TLSEnabled()
	}

	return res, nil
}

func (c *Config) Peers() (map[driver.PeerFunctionType][]*grpc.ConnectionConfig, error) {
	var connectionConfigs []*grpc.ConnectionConfig
	if err := c.configService.UnmarshalKey("fabric."+c.prefix+"peers", &connectionConfigs); err != nil {
		return nil, err
	}

	res := map[driver.PeerFunctionType][]*grpc.ConnectionConfig{}
	for _, v := range connectionConfigs {
		v.TLSEnabled = c.TLSEnabled()
		if v.TLSDisabled {
			v.TLSEnabled = false
		}
		usage := strings.ToLower(v.Usage)
		switch {
		case len(usage) == 0:
			res[driver.PeerForAnything] = append(res[driver.PeerForAnything], v)
		case usage == "delivery":
			res[driver.PeerForDelivery] = append(res[driver.PeerForDelivery], v)
		case usage == "discovery":
			res[driver.PeerForDiscovery] = append(res[driver.PeerForDiscovery], v)
		case usage == "finality":
			res[driver.PeerForFinality] = append(res[driver.PeerForFinality], v)
		case usage == "query":
			res[driver.PeerForQuery] = append(res[driver.PeerForQuery], v)
		default:
			logger.Warn("connection usage [%s] not recognized [%v]", usage, v)
		}
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

func (c *Config) VaultPersistencePrefix() string {
	return VaultPersistenceOptsKey
}

func (c *Config) VaultTXStoreCacheSize(defaultCacheSize int) int {
	v := c.configService.GetString("fabric." + c.prefix + "vault.txidstore.cache.size")
	cacheSize, err := strconv.Atoi(v)
	if err != nil {
		return defaultCacheSize
	}

	if cacheSize < 0 {
		return defaultCacheSize
	}

	return cacheSize
}

// DefaultMSP returns the default MSP
func (c *Config) DefaultMSP() string {
	return c.configService.GetString("fabric." + c.prefix + "defaultMSP")
}

func (c *Config) MSPs() ([]MSP, error) {
	var confs []MSP
	if err := c.configService.UnmarshalKey("fabric."+c.prefix+"msps", &confs); err != nil {
		return nil, err
	}
	return confs, nil
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

func (c *Config) MSPCacheSize() int {
	v := c.configService.GetString("fabric." + c.prefix + "mspCacheSize")
	if len(v) == 0 {
		return DefaultMSPCacheSize
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return DefaultMSPCacheSize
	}
	return i
}

func (c *Config) BroadcastNumRetries() int {
	v := c.configService.GetInt("fabric." + c.prefix + "ordering.numRetries")
	if v == 0 {
		return DefaultBroadcastNumRetries
	}
	return v
}

func (c *Config) BroadcastRetryInterval() time.Duration {
	return c.configService.GetDuration("fabric." + c.prefix + "ordering.retryInterval")
}

func (c *Config) OrdererConnectionPoolSize() int {
	v := c.configService.GetInt("fabric." + c.prefix + "ordering.connectionPoolSize")
	if v == 0 {
		v = DefaultOrderingConnectionPoolSize
	}
	return v
}
