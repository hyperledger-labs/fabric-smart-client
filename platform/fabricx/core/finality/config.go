/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"fmt"
	"time"
)

const DefaultRequestTimeout = 30 * time.Second

type Config struct {
	Endpoints      []Endpoint    `yaml:"endpoints,omitempty"`
	RequestTimeout time.Duration `yaml:"requestTimeout,omitempty"`
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
		RequestTimeout: DefaultRequestTimeout,
	}

	err := configService.UnmarshalKey("notificationService", &config)
	if err != nil {
		return config, fmt.Errorf("cannot get notify service config: %w", err)
	}

	return config, nil
}
