/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice

import (
	"fmt"
	"time"
)

const DefaultQueryTimeout = 30 * time.Second

type Config struct {
	Endpoints    []Endpoint    `yaml:"endpoints,omitempty"`
	QueryTimeout time.Duration `yaml:"queryTimeout,omitempty"`
}

type Endpoint struct {
	Address               string        `yaml:"address,omitempty"`
	ConnectionTimeout     time.Duration `yaml:"connectionTimeout,omitempty"`
	TLSEnabled            bool          `yaml:"tlsEnabled,omitempty"`
	TLSRootCertFile       string        `yaml:"tlsRootCertFile,omitempty"`
	TLSServerNameOverride string        `yaml:"tlsServerNameOverride,omitempty"`
}

type ConfigService interface {
	UnmarshalKey(key string, rawVal interface{}) error
}

func NewConfig(configService ConfigService) (*Config, error) {
	config := &Config{
		QueryTimeout: DefaultQueryTimeout,
	}

	err := configService.UnmarshalKey("queryService", &config)
	if err != nil {
		return config, fmt.Errorf("cannot get query service config: %w", err)
	}

	return config, nil
}
