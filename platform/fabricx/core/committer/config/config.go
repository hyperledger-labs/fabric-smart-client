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

// DefaultHandlerTimeout is the deadline set on the context handed to a single
// finality listener OnStatus callback. It is advisory: nothing can stop a running
// callback, so a listener that ignores cancellation occupies its worker for as long
// as it runs -- see DefaultHandlerWorkers.
const DefaultHandlerTimeout = 5 * time.Second

// DefaultHandlerWorkers is how many finality listener OnStatus callbacks may run
// concurrently. A callback that blocks forever occupies one worker for good, bounding a
// misbehaving listener's cost to throughput rather than unbounded goroutine growth --
// which is why OnStatus must observe its context and return promptly. It is a
// concurrency limit, not a rate limit: far larger batches are still delivered in full.
const DefaultHandlerWorkers = 16

// DefaultHandlerQueueSize is how many pending OnStatus invocations may be buffered while
// every worker is busy; it matches the generic committer's event queue
// (platform/common/core/generic/committer/finality.go). One response can carry far more
// transactions than there are workers, and without a buffer the surplus would wait for a
// sweep even with healthy listeners.
const DefaultHandlerQueueSize = 1000

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
		RequestTimeout:   DefaultRequestTimeout,
		HandlerTimeout:   DefaultHandlerTimeout,
		HandlerWorkers:   DefaultHandlerWorkers,
		HandlerQueueSize: DefaultHandlerQueueSize,
		ListenerTTL:      DefaultListenerTTL,
		SweepInterval:    DefaultSweepInterval,
	}
}

// Config holds the configuration for the gRPC client.
type Config struct {
	// Endpoints is a list of gRPC endpoints to connect to.
	Endpoints []Endpoint `yaml:"endpoints,omitempty"`
	// RequestTimeout is the timeout for gRPC requests.
	RequestTimeout time.Duration `yaml:"requestTimeout,omitempty"`
	// HandlerTimeout is the deadline set on the context handed to a single
	// finality listener OnStatus callback. Only meaningful for the notification
	// service. A value of zero falls back to DefaultHandlerTimeout.
	HandlerTimeout time.Duration `yaml:"handlerTimeout,omitempty"`
	// HandlerWorkers is how many finality listener OnStatus callbacks may run
	// concurrently. Only meaningful for the notification service. A value of zero
	// falls back to DefaultHandlerWorkers. Raise it when a deployment has
	// legitimately slow listeners; see DefaultHandlerWorkers for what happens when
	// every worker is occupied.
	HandlerWorkers int `yaml:"handlerWorkers,omitempty"`
	// HandlerQueueSize is how many pending OnStatus invocations may be buffered
	// while every worker is busy. Only meaningful for the notification
	// service. A value of zero falls back to DefaultHandlerQueueSize.
	HandlerQueueSize int `yaml:"handlerQueueSize,omitempty"`
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
// The returned Config is fully resolved: HandlerTimeout, HandlerWorkers,
// HandlerQueueSize, ListenerTTL and SweepInterval are pre-seeded with their
// defaults before unmarshaling, so a deployment that omits them keeps today's
// behavior. All of them except ListenerTTL also fall back to their defaults if
// explicitly set to zero -- unlike ListenerTTL, they have no "zero disables it"
// meaning, and a zero value would otherwise hand every listener an already-expired
// context or a pool that can never run anything. Callers can use every field
// as-is, with no further nil or zero-value handling.
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
	if config.HandlerWorkers <= 0 {
		config.HandlerWorkers = DefaultHandlerWorkers
	}
	if config.HandlerQueueSize <= 0 {
		config.HandlerQueueSize = DefaultHandlerQueueSize
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
