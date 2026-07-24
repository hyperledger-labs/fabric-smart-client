/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	commondriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	genericconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	viewconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

// ConfigProvider is the minimal read surface Config needs to discover and describe Fabric
// networks: the ability to unmarshal a configuration key into an arbitrary Go value.
type ConfigProvider interface {
	UnmarshalKey(key string, rawVal any) error
}

// DynamicConfigProvider is a ConfigProvider that also supports injecting new
// configuration into the live configuration tree at runtime.
type DynamicConfigProvider interface {
	ConfigProvider
	// MergeConfig merges the given raw yaml configuration into the current configuration.
	MergeConfig(raw []byte) error
	// ProvideFromRaw returns a new, independent provider whose configuration is loaded
	// from the given raw yaml representation.
	ProvideFromRaw(raw []byte) (*viewconfig.Provider, error)
}

// DynamicConfigService is a commondriver.ConfigService that also supports injecting new
// configuration into the live configuration tree at runtime. It satisfies DynamicConfigProvider.
type DynamicConfigService interface {
	commondriver.ConfigService
	// MergeConfig merges the given raw yaml configuration into the current configuration.
	MergeConfig(raw []byte) error
	// ProvideFromRaw returns a new, independent provider whose configuration is loaded
	// from the given raw yaml representation.
	ProvideFromRaw(raw []byte) (*viewconfig.Provider, error)
}

// FSNConfig is the per-network configuration read from the `fabric.<name>` configuration key.
type FSNConfig struct {
	// Default marks this network as the default one. At most one network may set this to true.
	Default bool `yaml:"default,omitempty"`
	// Driver is the name of the driver.Driver that should be used to instantiate this network.
	// If empty, every registered driver is tried in turn.
	Driver string `yaml:"driver,omitempty"`
}

// Config exposes the set of Fabric networks configured for this node: their names, which one
// is the default, and their per-network FSNConfig. It is safe for concurrent use; AddNetwork
// may add new networks to a live Config while other goroutines read from it.
type Config struct {
	mu sync.RWMutex

	provider       DynamicConfigProvider
	configurations map[string]*FSNConfig
	names          []string
	defaultName    string
}

// NewConfig builds a Config by scanning the `fabric` key of the given configProvider. It
// returns an error if no Fabric network is configured.
func NewConfig(configProvider DynamicConfigProvider) (*Config, error) {
	configurations, names, defaultName, err := loadNetworks(configProvider)
	if err != nil {
		return nil, err
	}

	return &Config{
		provider:       configProvider,
		names:          names,
		defaultName:    defaultName,
		configurations: configurations,
	}, nil
}

// loadNetworks scans the `fabric` key of the given provider and returns the discovered
// per-network configurations, the ordered (sorted) list of network names, and the resolved
// default network name. Every network name and channel name it discovers is validated against
// the Fabric naming rules, and at most one network may be marked as default (explicitly via
// FSNConfig.Default, or implicitly by being named "default"); violating either rule is an error.
func loadNetworks(configProvider ConfigProvider) (map[string]*FSNConfig, []string, string, error) {
	var value any
	if err := configProvider.UnmarshalKey("fabric", &value); err != nil {
		return nil, nil, "", errors.Wrap(err, "failed unmarshalling `fabric` key")
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, nil, "", errors.New("failed unmarshalling `fabric` key: expected a map")
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		if strings.ToLower(k) == "enabled" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var names []string
	configurations := map[string]*FSNConfig{}
	for _, name := range keys {
		if err := validateNetworkName(name); err != nil {
			return nil, nil, "", errors.Wrapf(err, "invalid fabric network [%s]", name)
		}

		var fsnConfig FSNConfig
		if err := configProvider.UnmarshalKey("fabric."+name, &fsnConfig); err != nil {
			return nil, nil, "", errors.Wrapf(err, "failed unmarshalling `fabric.%s` key", name)
		}
		if err := validateChannels(configProvider, name); err != nil {
			return nil, nil, "", errors.Wrapf(err, "invalid fabric network [%s]", name)
		}

		names = append(names, name)
		configurations[name] = &fsnConfig
		logger.Debugf("found fabric network [%s], driver [%s]", name, fsnConfig.Driver)
	}
	if len(names) == 0 {
		return nil, nil, "", errors.New("no fabric network configured")
	}

	defaults := explicitDefaults(configurations, names)
	if len(defaults) > 1 {
		return nil, nil, "", errors.Errorf("multiple fabric networks are set as default: %v", defaults)
	}

	var defaultName string
	if len(defaults) == 1 {
		defaultName = defaults[0]
	} else {
		defaultName = names[0]
		if len(names) > 1 {
			logger.Warnf("no default network configured, set it to [%s]", defaultName)
		}
	}

	return configurations, names, defaultName, nil
}

// explicitDefaults returns, in sorted order, the names among names that explicitly request to
// be the default network: either FSNConfig.Default is set, or the network is named "default".
func explicitDefaults(configurations map[string]*FSNConfig, names []string) []string {
	var defaults []string
	for _, name := range names {
		if configurations[name].Default || name == "default" {
			defaults = append(defaults, name)
		}
	}
	sort.Strings(defaults)
	return defaults
}

// Names returns the names of every currently configured Fabric network, including any added
// at runtime via AddNetwork.
func (c *Config) Names() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.names
}

// DefaultName returns the name of the default Fabric network.
func (c *Config) DefaultName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.defaultName
}

// Config returns the FSNConfig for the given network name, or an error if no such network is
// configured.
func (c *Config) Config(network string) (*FSNConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conf, ok := c.configurations[network]
	if !ok {
		return nil, errors.Errorf("cannot find configuration for network [%s]", network)
	}
	return conf, nil
}

// NetworkConfigValidator validates a candidate Fabric network's configuration before it is
// merged into the live configuration tree. It is invoked once per network name introduced by
// an AddNetwork call, with a ConfigProvider scoped to the parsed, not-yet-merged incoming
// configuration, so a validator can inspect any key under `fabric.<networkName>.*`.
type NetworkConfigValidator interface {
	Validate(networkName string, configProvider ConfigProvider) error
}

// NetworkConfigValidatorFunc adapts a function to a NetworkConfigValidator.
type NetworkConfigValidatorFunc func(networkName string, configProvider ConfigProvider) error

// Validate calls f.
func (f NetworkConfigValidatorFunc) Validate(networkName string, configProvider ConfigProvider) error {
	return f(networkName, configProvider)
}

// AddNetwork validates the given raw yaml configuration and, if it describes one or more new
// Fabric networks, merges it into the live configuration tree.
//
// The raw configuration must be a valid `fabric` configuration snippet (e.g. `fabric.<name>.*`
// keys). Every network name and channel name it introduces is validated against the Fabric
// naming rules (the same rules applied to the configuration loaded at startup), and every
// network it introduces must be new: adding a network whose name already exists is rejected, as
// updating an existing network's configuration is not supported. A network requesting to become
// the default (via FSNConfig.Default or the name "default") is also rejected if doing so would
// leave more than one network marked as default. Any additional validators are run, per
// incoming network, after the built-in checks and before anything is merged.
func (c *Config) AddNetwork(raw []byte, validators ...NetworkConfigValidator) error {
	temp, err := c.provider.ProvideFromRaw(raw)
	if err != nil {
		return errors.Wrap(err, "failed parsing fabric configuration")
	}

	incomingConfigurations, incomingNames, _, err := loadNetworks(temp)
	if err != nil {
		return errors.Wrap(err, "failed loading fabric configuration")
	}

	for _, name := range incomingNames {
		for _, v := range validators {
			if err := v.Validate(name, temp); err != nil {
				return errors.Wrapf(err, "custom validation failed for fabric network [%s]", name)
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, name := range incomingNames {
		if _, exists := c.configurations[name]; exists {
			return errors.Errorf("fabric network [%s] already exists, updating an existing network is not supported", name)
		}
	}

	defaults := append(explicitDefaults(c.configurations, c.names), explicitDefaults(incomingConfigurations, incomingNames)...)
	sort.Strings(defaults)
	if len(defaults) > 1 {
		return errors.Errorf("multiple fabric networks are set as default: %v", defaults)
	}

	if err := c.provider.MergeConfig(raw); err != nil {
		return errors.Wrap(err, "failed merging fabric configuration")
	}

	configurations, names, defaultName, err := loadNetworks(c.provider)
	if err != nil {
		return errors.Wrap(err, "failed reloading fabric configuration after merge")
	}
	c.configurations = configurations
	c.names = names
	c.defaultName = defaultName

	logger.Infof("added fabric network(s) %v", incomingNames)

	return nil
}

// identifierAllowedChars matches the Fabric convention for channel/network identifiers: a
// lowercase letter followed by any number of lowercase alphanumerics, dots, and dashes.
var identifierAllowedChars = regexp.MustCompile(`^[a-z][a-z0-9.-]*$`)

// maxIdentifierLength is the maximum length allowed for a network or channel name, matching the
// Fabric channel ID convention.
const maxIdentifierLength = 249

// validateIdentifier checks that name complies with the Fabric naming rules for identifiers:
// contains only lowercase ASCII alphanumerics, dots, and dashes; starts with a letter; and is
// shorter than maxIdentifierLength characters.
func validateIdentifier(name string) error {
	if len(name) == 0 {
		return errors.New("identifier illegal, cannot be empty")
	}
	if len(name) > maxIdentifierLength {
		return errors.Errorf("identifier illegal, cannot be longer than %d", maxIdentifierLength)
	}
	if !identifierAllowedChars.MatchString(name) {
		return errors.Errorf("identifier [%s] contains illegal characters", name)
	}
	return nil
}

// validateNetworkName checks that the given network name complies with the Fabric naming
// rules for identifiers (lowercase alphanumerics, dots and dashes, starting with a letter).
func validateNetworkName(name string) error {
	if err := validateIdentifier(name); err != nil {
		return errors.Wrapf(err, "invalid network name [%s]", name)
	}
	return nil
}

// validateChannels checks that every channel declared for the given network under the given
// provider has a name that complies with the Fabric naming rules, and that no two channels of
// the same network share the same name.
func validateChannels(configProvider ConfigProvider, network string) error {
	var channels []*genericconfig.Channel
	if err := configProvider.UnmarshalKey("fabric."+network+".channels", &channels); err != nil {
		return errors.Wrapf(err, "failed unmarshalling channels for network [%s]", network)
	}

	seen := map[string]struct{}{}
	for _, ch := range channels {
		if ch == nil || len(ch.Name) == 0 {
			return errors.Errorf("channel name is empty for network [%s]", network)
		}
		if err := validateIdentifier(ch.Name); err != nil {
			return errors.Wrapf(err, "invalid channel name [%s] for network [%s]", ch.Name, network)
		}
		if _, exists := seen[ch.Name]; exists {
			return errors.Errorf("duplicate channel name [%s] for network [%s]", ch.Name, network)
		}
		seen[ch.Name] = struct{}{}
	}

	return nil
}
