/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	koanfyaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	koanffile "github.com/knadh/koanf/providers/file"
	koanfbytes "github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

const (
	CmdRoot = "core"
	// IDKey is the key to retrieve the FSC id
	IDKey = "fsc.id"
	// EnvCorePrefix is the prefix for the env variables
	EnvCorePrefix = "CORE_"
)

const (
	OfficialPath          = "/etc/hyperledger-labs/fabric-smart-client-node"
	MergeConfigEventTopic = "fsc.mergeConfig.event.topic"
)

var (
	logOutput = os.Stderr
	logger    = logging.MustGetLogger()
)

type OnMergeConfigEventHandler interface {
	OnMergeConfig()
}

type DecodeHookFuncType func(reflect.Type, reflect.Type, interface{}) (interface{}, error)

type Provider struct {
	Backend     *koanf.Koanf
	eventSystem events.EventSystem

	mergeConfigMutex sync.Mutex
	fullPath         string
}

// NewProvider creates a new configuration provider from the given path.
func NewProvider(confPath string) (*Provider, error) {
	p := &Provider{
		eventSystem: simple.NewEventBus(),
	}
	if err := p.loadFromPath(confPath); err != nil {
		return nil, err
	}

	return p, nil
}

// GetProvider returns an instance of the config service.
// It panics, if no instance is found.
func GetProvider(sp services.Provider) *Provider {
	s, err := sp.GetService(reflect.TypeOf((*Provider)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(*Provider)
}

// ID returns the FSC node ID from the configuration.
func (p *Provider) ID() string {
	return p.GetString(IDKey)
}

// GetDuration returns the duration value associated with the given key.
func (p *Provider) GetDuration(key string) time.Duration {
	return p.Backend.Duration(strings.ToLower(key))
}

// GetBool returns the boolean value associated with the given key.
func (p *Provider) GetBool(key string) bool {
	return p.Backend.Bool(strings.ToLower(key))
}

// GetInt returns the integer value associated with the given key.
func (p *Provider) GetInt(key string) int {
	return p.Backend.Int(strings.ToLower(key))
}

// GetStringSlice returns the string slice associated with the given key.
func (p *Provider) GetStringSlice(key string) []string {
	return p.Backend.Strings(strings.ToLower(key))
}

// UnmarshalKey unmarshals the configuration value associated with the given key into the raw value.
func (p *Provider) UnmarshalKey(key string, rawVal interface{}) error {
	return EnhancedExactUnmarshal(p.Backend, strings.ToLower(key), rawVal)
}

// IsSet returns true if the given key is set in the configuration.
func (p *Provider) IsSet(key string) bool {
	return p.Backend.Exists(strings.ToLower(key))
}

// GetPath returns the translated path associated with the given key.
func (p *Provider) GetPath(key string) string {
	path := p.Backend.String(strings.ToLower(key))
	if path == "" {
		return ""
	}

	return TranslatePath(filepath.Dir(p.fullPath), path)
}

// TranslatePath returns the translated path.
func (p *Provider) TranslatePath(path string) string {
	if path == "" {
		return ""
	}

	return TranslatePath(filepath.Dir(p.fullPath), path)
}

// GetString returns the string value associated with the given key.
func (p *Provider) GetString(key string) string {
	logger.Infof("Get string [%s]", key)
	return p.Backend.String(strings.ToLower(key))
}

// ConfigFileUsed returns the configuration file used.
// Currently, this returns an empty string as koanf does not track it directly.
func (p *Provider) ConfigFileUsed() string {
	// koanf does not track the config file used, so we return empty string
	return ""
}

// MergeConfig merges the given raw configuration into the current configuration.
func (p *Provider) MergeConfig(raw []byte) error {
	// only one writer at the time
	p.mergeConfigMutex.Lock()
	defer p.mergeConfigMutex.Unlock()

	// Load the raw config into a temporary koanf
	tmp := koanf.New(".")
	if err := tmp.Load(koanfbytes.Provider(raw), LowercaseParser{Parser: koanfyaml.Parser()}); err != nil {
		return err
	}
	// Load the merged map into p.Backend
	if err := p.Backend.Merge(tmp); err != nil {
		return err
	}

	// notify the listener
	p.eventSystem.Publish(&MergeConfigEvent{})

	return nil
}

// OnMergeConfig registers an event handler to be notified when the configuration is merged.
func (p *Provider) OnMergeConfig(handler OnMergeConfigEventHandler) {
	p.eventSystem.Subscribe(MergeConfigEventTopic, &eventListener{handler: handler})
}

// String returns a string representation of the configuration.
func (p *Provider) String() string {
	out, err := p.Backend.Marshal(koanfyaml.Parser())
	if err != nil {
		return err.Error()
	}
	return string(out)
}

// ProvideFromRaw returns a new Provider whose configuration is loaded from the given byte representation.
// The function expects a valid `yaml` representation.
func (p *Provider) ProvideFromRaw(raw []byte) (*Provider, error) {
	newProvider := &Provider{
		eventSystem: simple.NewEventBus(),
	}
	if err := newProvider.loadFromRaw(raw); err != nil {
		return nil, err
	}
	// koanf does not have a SetConfigFile method, so we skip setting the config file.

	return newProvider, nil
}

func (p *Provider) loadFromPath(path string) error {
	p.Backend = koanf.New(".")
	paths, err := p.initConfigPaths(path)
	if err != nil {
		return err
	}

	var loadErr error
	for _, pth := range paths {
		fullPath := filepath.Join(pth, CmdRoot+".yaml")
		if loadErr = p.Backend.Load(koanffile.Provider(fullPath), LowercaseParser{Parser: koanfyaml.Parser()}); loadErr == nil {
			// found and loaded successfully
			fp, err := filepath.Abs(fullPath)
			if err != nil {
				return err
			}
			p.fullPath = fp
			break
		}
	}
	if loadErr != nil {
		return errors.Errorf("Could not find config file. "+
			"Please make sure that FSCNODE_CFG_PATH is set to a path "+
			"which contains %s.yaml", CmdRoot)
	}
	if err := p.setupEnv(); err != nil {
		return err
	}

	logging.Init(logging.Config{
		Format:       p.Backend.String("logging.format"),
		LogSpec:      p.Backend.String("logging.spec"),
		OtelSanitize: p.Backend.Bool("logging.otel.sanitize"),
		Writer:       logOutput,
	})

	return nil
}

func (p *Provider) loadFromRaw(raw []byte) error {
	p.Backend = koanf.New(".")
	// No need to set config type

	// read configuration
	if err := p.Backend.Load(koanfbytes.Provider(raw), LowercaseParser{Parser: koanfyaml.Parser()}); err != nil {
		return errors.Wrapf(err, "failed to read configuration from raw [%s]", logging.SHA256Base64(raw))
	}
	// post process
	if err := p.setupEnv(); err != nil {
		return err
	}

	return nil
}

func (p *Provider) setupEnv() error {
	// Load only environment variables with prefix "CORE_" and merge into config.
	// Transform var names by:
	// 1. Converting to lowercase
	// 2. Removing "MYVAR_" prefix
	// 3. Replacing "_" with "." to representing nesting using the . delimiter.
	// Example: MYVAR_PARENT1_CHILD1_NAME becomes "parent1.child1.name"
	if err := p.Backend.Load(env.Provider(".", env.Opt{
		Prefix: "CORE",
		TransformFunc: func(k, v string) (string, any) {
			if len(v) == 0 {
				// discard this
				return "", ""
			}

			// Transform the key.
			k = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(k, EnvCorePrefix)), "_", ".")

			// Get the existing value from koanf to check its type
			existingValue := p.Backend.Get(k)

			// SCENARIO 2: Prevent the env var (which is a string) from changing
			// the type of an existing int, float, or bool.
			switch existingValue.(type) {
			case map[string]any:
				// should not replace
				return "", existingValue
			case int:
				// Convert the env string to an int so the type is preserved
				if parsed, err := strconv.Atoi(v); err == nil {
					return k, parsed
				}
			case bool:
				// Convert to bool
				if parsed, err := strconv.ParseBool(v); err == nil {
					return k, parsed
				}
			case float64:
				// Convert to float64
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					return k, parsed
				}
			}
			return k, v
		},
	}), nil); err != nil {
		return err
	}
	return nil
}

// ----------------------------------------------------------------------------------
// InitKoanf()
// ----------------------------------------------------------------------------------
// Performs basic initialization of our koanf-based configuration layer.
// Primary thrust is to establish the paths that should be consulted to find
// the configuration we need.  If Backend == nil, we will initialize the global
// koanf instance
// ----------------------------------------------------------------------------------
func (p *Provider) initConfigPaths(confPath string) ([]string, error) {
	var paths []string

	if len(confPath) != 0 {
		paths = append(paths, confPath)
	}

	var altPath = os.Getenv("FSCNODE_CFG_PATH")
	if altPath != "" {
		// If the user has overridden the path with an envvar, its the only path
		// we will consider

		if !dirExists(altPath) {
			return nil, errors.Errorf("FSCNODE_CFG_PATH %s does not exist", altPath)
		}
		paths = []string{altPath} // override, only this path
	} else {
		// If we get here, we should use the default paths in priority order:
		//
		// *) CWD
		// *) /etc/hyperledger/fsc

		// CWD
		paths = append(paths, "./")

		// And finally, the official path
		if dirExists(OfficialPath) {
			paths = append(paths, OfficialPath)
		}
	}

	return paths, nil
}

// SetConfigPath sets the configuration path.
func (p *Provider) SetConfigPath(path string) error {
	fp, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	p.fullPath = fp

	return nil
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

// TranslatePath joins the base path and the given path if the given path is relative.
func TranslatePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}

	return filepath.Join(base, p)
}

type eventListener struct {
	handler OnMergeConfigEventHandler
}

// OnReceive is called when a merge configuration event is received.
func (e *eventListener) OnReceive(event events.Event) {
	e.handler.OnMergeConfig()
}

type MergeConfigEvent struct {
}

// Topic returns the merge configuration event topic.
func (m *MergeConfigEvent) Topic() string {
	return MergeConfigEventTopic
}

// Message returns the merge configuration event message.
func (m *MergeConfigEvent) Message() interface{} {
	return nil
}

// LowercaseParser wraps an existing parser to lowercase all keys
type LowercaseParser struct {
	koanf.Parser
}

// Unmarshal unmarshals the configuration into a map and lowercases the keys.
func (l LowercaseParser) Unmarshal(b []byte) (map[string]any, error) {
	m, err := l.Parser.Unmarshal(b)
	if err != nil {
		return nil, err
	}
	return lowercaseMapKeys(m), nil
}

// lowercaseMapKeys recursively converts all map keys to lowercase
func lowercaseMapKeys(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for key, val := range m {
		lowerKey := strings.ToLower(key)
		if nestedMap, ok := val.(map[string]any); ok {
			out[lowerKey] = lowercaseMapKeys(nestedMap)
		} else if nestedSlice, ok := val.([]any); ok {
			for i, nested := range nestedSlice {
				if ns, ok := nested.(map[string]any); ok {
					nestedSlice[i] = lowercaseMapKeys(ns)
				}
			}
			out[lowerKey] = val
		} else {
			out[lowerKey] = val
		}
	}
	return out
}
