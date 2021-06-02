/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type ConfigService interface {
	GetBool(s string) bool
	GetString(s string) string
	GetDuration(s string) time.Duration
	GetPath(s string) string
	UnmarshalKey(s string, i interface{}) error
	TranslatePath(path string) string
}

type Config struct {
	configService ConfigService
}

func NewConfig(configService ConfigService) *Config {
	return &Config{
		configService: configService,
	}
}

func (c *Config) TLSEnabled() bool {
	return c.configService.GetBool("fabric.tls.enabled")
}

func (c *Config) TLSClientAuthRequired() bool {
	return c.configService.GetBool("fabric.tls.clientAuthRequired")
}

func (c *Config) TLSServerHostOverride() string {
	return c.configService.GetString("fabric.tls.serverhostoverride")
}

func (c *Config) ClientConnTimeout() time.Duration {
	return c.configService.GetDuration("fabric.client.connTimeout")
}

func (c *Config) TLSClientKeyFile() string {
	return c.configService.GetPath("fabric.tls.clientKey.file")
}

func (c *Config) TLSClientCertFile() string {
	return c.configService.GetPath("fabric.tls.clientCert.file")
}

func (c *Config) TLSRootCertFile() string {
	return c.configService.GetString("fabric.tls.rootCertFile")
}

func (c *Config) Orderers() ([]*grpc.ConnectionConfig, error) {
	var res []*grpc.ConnectionConfig
	if err := c.configService.UnmarshalKey("fabric.orderers", &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Config) Peers() ([]*grpc.ConnectionConfig, error) {
	var res []*grpc.ConnectionConfig
	if err := c.configService.UnmarshalKey("fabric.peers", &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Config) Channels() ([]*Channel, error) {
	var res []*Channel
	if err := c.configService.UnmarshalKey("fabric.channels", &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Config) VaultPersistenceType() string {
	return c.configService.GetString("fabric.vault.persistence.type")
}

func (c *Config) VaultPersistenceOpts(opts interface{}) error {
	return c.configService.UnmarshalKey("fabric.vault.persistence.opts", opts)
}

func (c *Config) MSPConfigPath() string {
	return c.configService.GetPath("fabric.mspConfigPath")
}

func (c *Config) MSPs(confs *[]msp.Configuration) error {
	return c.configService.UnmarshalKey("fabric.msps", confs)
}

// LocalMSPID returns the local MSP ID
func (c *Config) LocalMSPID() string {
	return c.configService.GetString("fabric.localMspId")
}

// LocalMSPType returns the local MSP Type
func (c *Config) LocalMSPType() string {
	return c.configService.GetString("fabric.localMspType")
}

// TranslatePath translates the passed path relative to the path from which the configuration has been loaded
func (c *Config) TranslatePath(path string) string {
	return c.configService.TranslatePath(path)
}
