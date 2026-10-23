/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"slices"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// Keys of the remote view API: the view service as reachable from other processes, on the
// transports (backends) listed in fsc.view.remote.backends.
const (
	RemoteKey               = "fsc.view.remote"
	RemoteEnabledKey        = "fsc.view.remote.enabled"
	RemoteTimeWindowKey     = "fsc.view.remote.timeWindow"
	RemoteMaxMessageSizeKey = "fsc.view.remote.maxMessageSize"
	RemoteBackendsKey       = "fsc.view.remote.backends"

	// DefaultCommandTimeWindow is the default fsc.view.remote.timeWindow.
	DefaultCommandTimeWindow = 15 * time.Minute
)

// RemoteConfig is the configuration of the remote view API.
type RemoteConfig struct {
	// Enabled reports whether the remote view API is served at all.
	Enabled bool
	// TimeWindow is how far a command's timestamp may be from the local clock.
	TimeWindow time.Duration
	// MaxMessageSize is the largest command or stream frame accepted, in bytes; 0 is unset.
	MaxMessageSize int
	// Backends names the transports the remote view API is served on, in mount order.
	Backends []string
}

// LoadRemoteConfig reads and checks fsc.view.remote. The remote view API is off unless
// fsc.view.remote.enabled is true, and is never inferred from a listener's enabled flag. Two
// checks run regardless of it, so a configuration does not silently serve no view API: any
// fsc.view.remote key other than enabled without enabled: true, and fsc.grpc.enabled: true without
// fsc.view.remote (nothing else uses the gRPC listener). With enabled: true, every key must be
// known, so a misspelt key fails instead of leaving its default in place, backends must be
// non-empty and free of duplicates, a listed built-in backend needs its listener enabled,
// timeWindow must be a positive duration and maxMessageSize a positive integer. Every error
// names the key. Whether a listed name is an installed backend is checked when the backends
// are mounted.
func LoadRemoteConfig(c driver.ConfigService) (RemoteConfig, error) {
	cfg := RemoteConfig{TimeWindow: DefaultCommandTimeWindow}
	if !c.GetBool(RemoteEnabledKey) {
		if remoteKeysBesidesEnabled(c) {
			return cfg, errors.Errorf("[%s] is set but [%s] is not true; set it to serve the remote view API, or remove [%s]", RemoteKey, RemoteEnabledKey, RemoteKey)
		}
		if c.GetBool("fsc.grpc.enabled") {
			return cfg, errors.Errorf("[fsc.grpc.enabled] is true but the remote view API is not configured: set [%s] to true with backends: [grpc], or disable fsc.grpc", RemoteEnabledKey)
		}
		return cfg, nil
	}
	cfg.Enabled = true
	if unknown := unknownRemoteKeys(c); len(unknown) > 0 {
		return cfg, errors.Errorf("[%s] has unknown keys %v; known keys are enabled, backends, timeout, timeWindow, clients and maxMessageSize", RemoteKey, unknown)
	}

	if c.IsSet(RemoteTimeWindowKey) {
		raw := c.GetString(RemoteTimeWindowKey)
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return cfg, errors.Errorf("[%s] must be a positive duration, got [%s]", RemoteTimeWindowKey, raw)
		}
		cfg.TimeWindow = d
	}
	if c.IsSet(RemoteMaxMessageSizeKey) {
		if cfg.MaxMessageSize = c.GetInt(RemoteMaxMessageSizeKey); cfg.MaxMessageSize <= 0 {
			return cfg, errors.Errorf("[%s] must be a positive number of bytes, got [%s]", RemoteMaxMessageSizeKey, c.GetString(RemoteMaxMessageSizeKey))
		}
	}

	cfg.Backends = c.GetStringSlice(RemoteBackendsKey)
	if len(cfg.Backends) == 0 {
		return cfg, errors.Errorf("[%s] is true but [%s] lists no transport", RemoteEnabledKey, RemoteBackendsKey)
	}
	if c.GetBool("fsc.grpc.enabled") && !slices.Contains(cfg.Backends, "grpc") {
		// A custom backend may use the gRPC listener, so this is not an error.
		logger.Warnf("[fsc.grpc.enabled] is true but [%s] does not list grpc; the gRPC listener serves only what custom backends mount on it", RemoteBackendsKey)
	}
	for i, name := range cfg.Backends {
		if slices.Contains(cfg.Backends[:i], name) {
			return cfg, errors.Errorf("[%s] lists [%s] twice", RemoteBackendsKey, name)
		}
		if listener, builtIn := builtInBackendListeners[name]; builtIn && !c.GetBool(listener) {
			return cfg, errors.Errorf("[%s] lists [%s] but [%s] is false", RemoteBackendsKey, name, listener)
		}
	}
	return cfg, nil
}

// remoteKeys are the keys of fsc.view.remote, lowercased as the config provider stores them.
var remoteKeys = []string{"enabled", "backends", "timeout", "timewindow", "clients", "maxmessagesize"}

// unknownRemoteKeys returns the keys of fsc.view.remote that are not in remoteKeys, sorted.
func unknownRemoteKeys(c driver.ConfigService) []string {
	tree, _ := c.RawSubtree(RemoteKey)
	var unknown []string
	for k := range tree {
		if !slices.Contains(remoteKeys, k) {
			unknown = append(unknown, k)
		}
	}
	slices.Sort(unknown)
	return unknown
}

// remoteKeysBesidesEnabled reports whether fsc.view.remote sets any key other than enabled.
func remoteKeysBesidesEnabled(c driver.ConfigService) bool {
	tree, ok := c.RawSubtree(RemoteKey)
	if !ok {
		return false
	}
	for k := range tree {
		if k != "enabled" {
			return true
		}
	}
	return len(tree) == 0
}

// builtInBackendListeners maps each built-in backend to the enabled flag of its listener.
var builtInBackendListeners = map[string]string{
	"grpc": "fsc.grpc.enabled",
	"web":  "fsc.web.enabled",
}
