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
	// TLSEnabled indicates whether TLS is enabled for this endpoint.
	TLSEnabled bool `yaml:"tlsEnabled,omitempty"`
	// TLSRootCertFile is the path to the TLS root certificate file.
	TLSRootCertFile string `yaml:"tlsRootCertFile,omitempty"`
	// TLSServerNameOverride is the server name to use for TLS hostname verification.
	TLSServerNameOverride string `yaml:"tlsServerNameOverride,omitempty"`
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
