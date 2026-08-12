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

// DefaultHandlerTimeout is the maximum time allowed for a single finality
// listener OnStatus callback to run before it is abandoned. If a handler
// exceeds this timeout, a warning is logged and the handler is abandoned.
// Note: handlers that ignore context cancellation will leak goroutines, but
// this is preferable to blocking the dispatcher.
const DefaultHandlerTimeout = 5 * time.Second

// DefaultListenerTTL bounds how long a finality listener may wait locally for
// a notification that may never arrive before being settled with Unknown. It
// is deliberately much longer than RequestTimeout: that timeout is documented
// non-strict ("it is possible to receive notifications after the timeout has
// passed", see notify.proto), so the remote must be given ample room to
// answer before the listener gives up locally. Local expiry is a backstop
// against silence, not a competitor to the remote deadline.
const DefaultListenerTTL = 2 * time.Minute

// DefaultSweepInterval is how often expired finality listener entries are
// collected. An entry's worst-case lifetime is ListenerTTL + SweepInterval.
const DefaultSweepInterval = 30 * time.Second

// DefaultConfig returns a Config with every field set to its documented
// default. Use it where no ServiceBackend is available (e.g. a caller that
// never resolves configuration for a real network) but a fully-defaulted
// Config is still required.
func DefaultConfig() Config {
	return Config{
		RequestTimeout: DefaultRequestTimeout,
		HandlerTimeout: DefaultHandlerTimeout,
		ListenerTTL:    DefaultListenerTTL,
		SweepInterval:  DefaultSweepInterval,
	}
}

// Config holds the configuration for the gRPC client.
type Config struct {
	// Endpoints is a list of gRPC endpoints to connect to.
	Endpoints []Endpoint `yaml:"endpoints,omitempty"`
	// RequestTimeout is the timeout for gRPC requests.
	RequestTimeout time.Duration `yaml:"requestTimeout,omitempty"`
	// HandlerTimeout is the maximum time allowed for a single finality listener
	// OnStatus callback to run before it is abandoned. Only meaningful for the
	// notification service. A value of zero falls back to DefaultHandlerTimeout.
	HandlerTimeout time.Duration `yaml:"handlerTimeout,omitempty"`
	// ListenerTTL bounds how long a finality listener may wait locally for a
	// notification before being settled with Unknown. Only meaningful for the
	// notification service. Explicitly setting it to zero disables local expiry;
	// leaving it unset falls back to DefaultListenerTTL.
	ListenerTTL time.Duration `yaml:"listenerTTL,omitempty"`
	// SweepInterval is how often expired finality listener entries are
	// collected. Only meaningful for the notification service. A value of zero
	// falls back to DefaultSweepInterval.
	SweepInterval time.Duration `yaml:"sweepInterval,omitempty"`
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
	// RootCertPaths are the paths to TLS root certificates.
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
	UnmarshalKey(key string, rawVal any) error
}

// NewNotificationServiceConfig creates a new Config instance by unmarshaling the "notificationService" key
// from the provided ServiceBackend. It returns an error if the unmarshaling fails.
//
// The returned Config is fully resolved: HandlerTimeout, ListenerTTL and
// SweepInterval are pre-seeded with their defaults before unmarshaling, so a
// deployment that omits them keeps today's behavior, and HandlerTimeout /
// SweepInterval fall back to their defaults if explicitly set to zero too --
// unlike ListenerTTL, they have no "zero disables it" meaning, so a zero value
// would otherwise hand every listener an already-expired context. Callers can
// use every field as-is, with no further nil or zero-value handling.
func NewNotificationServiceConfig(configService ServiceBackend) (*Config, error) {
	defaults := DefaultConfig()
	config := &defaults

	err := configService.UnmarshalKey("notificationService", &config)
	if err != nil {
		return config, errors.Wrap(err, "unmarshal notificationService")
	}

	if config.HandlerTimeout <= 0 {
		config.HandlerTimeout = DefaultHandlerTimeout
	}
	if config.SweepInterval <= 0 {
		config.SweepInterval = DefaultSweepInterval
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
