/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// DefaultRequestTimeout is the default timeout for gRPC requests.
const DefaultRequestTimeout = 30 * time.Second

// Config holds the configuration for the gRPC client.
type Config struct {
	// Endpoints is a list of gRPC endpoints to connect to.
	Endpoints []Endpoint `yaml:"endpoints,omitempty"`
	// RequestTimeout is the timeout for gRPC requests.
	RequestTimeout time.Duration `yaml:"requestTimeout,omitempty"`
}

// Endpoint describes a single gRPC endpoint.
type Endpoint struct {
	// Address is the host:port of the gRPC service.
	Address string `yaml:"address,omitempty"`
	// ConnectionTimeout is the timeout for establishing a connection.
	ConnectionTimeout time.Duration `yaml:"connectionTimeout,omitempty"`
	// TLS is the TLS configuration for the given endpoint.
	TLS *TLSConfig `yaml:"tls,omitempty"`
}

// TLSConfig specifies TLS settings for secure communication.
// Supports three modes: no TLS, server TLS (rootCerts only), and mutual TLS (all fields).
type TLSConfig struct {
	// Enabled indicates whether TLS is enabled for this endpoint.
	Enabled bool `yaml:"enabled"`
	// ClientKeyPath is the path to TLS client private key
	ClientKeyPath string `yaml:"clientKey,omitempty"`
	// ClientCertPath is the path to TLS client certificate
	ClientCertPath string `yaml:"clientCert,omitempty"`
	// RootCertPaths are the pats to TLS root certificates.
	RootCertPaths []string `yaml:"rootCerts,omitempty"`
	// ServerNameOverride is the server name to use for TLS hostname verification.
	ServerNameOverride string `yaml:"serverNameOverride,omitempty"`
}

// IsEnabled returns whether TLS is enabled for this configuration.
// Returns false if the config is nil or the enabled flag false.
func (c *TLSConfig) IsEnabled() bool {
	if c == nil {
		return false // default
	}
	return c.Enabled
}

// ServiceBackend defines the interface for retrieving configuration values.
//
//go:generate counterfeiter -o mock/service_backend.go --fake-name ServiceBackend . ServiceBackend
type ServiceBackend interface {
	// UnmarshalKey takes a single key and unmarshal it into a struct.
	UnmarshalKey(key string, rawVal interface{}) error
}

// NewNotificationServiceConfig creates a new Config instance by unmarshaling the "notificationService" key
// from the provided ServiceBackend. It returns an error if the unmarshaling fails.
func NewNotificationServiceConfig(configService ServiceBackend) (*Config, error) {
	config := &Config{
		RequestTimeout: DefaultRequestTimeout,
	}

	err := configService.UnmarshalKey("notificationService", &config)
	if err != nil {
		return config, errors.Wrap(err, "unmarshal notificationService")
	}

	return config, nil
}

// NewQueryServiceConfig creates a new Config instance by unmarshaling the "queryService" key
// from the provided ServiceBackend. It returns an error if the unmarshaling fails.
func NewQueryServiceConfig(configService ServiceBackend) (*Config, error) {
	config := &Config{
		RequestTimeout: DefaultRequestTimeout,
	}

	err := configService.UnmarshalKey("queryService", &config)
	if err != nil {
		return config, errors.Wrap(err, "unmarshal queryService")
	}

	return config, nil
}
