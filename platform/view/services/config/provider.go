/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	viperutil "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config/viper"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

const (
	CmdRoot = "core"
	// IDKey is the key to retrieve the FSC id
	IDKey = "fsc.id"
)

const OfficialPath = "/etc/hyperledger-labs/fabric-smart-client-node"

var logOutput = os.Stderr

type DecodeHookFuncType func(reflect.Type, reflect.Type, interface{}) (interface{}, error)

type Provider struct {
	confPath string
	v        *viper.Viper
}

func NewProvider(confPath string) (*Provider, error) {
	p := &Provider{confPath: confPath}
	if err := p.load(); err != nil {
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

func (p *Provider) ID() string {
	return p.GetString(IDKey)
}

func (p *Provider) GetDuration(key string) time.Duration {
	return p.v.GetDuration(key)
}

func (p *Provider) GetBool(key string) bool {
	return p.v.GetBool(key)
}

func (p *Provider) GetInt(key string) int {
	return p.v.GetInt(key)
}

func (p *Provider) GetStringSlice(key string) []string {
	return p.v.GetStringSlice(key)
}

func (p *Provider) AddDecodeHook(f DecodeHookFuncType) error {
	return nil
}

func (p *Provider) UnmarshalKey(key string, rawVal interface{}) error {
	return viperutil.EnhancedExactUnmarshal(p.v, key, rawVal)
}

func (p *Provider) IsSet(key string) bool {
	return p.v.IsSet(key)
}

func (p *Provider) GetPath(key string) string {
	path := p.v.GetString(key)
	if path == "" {
		return ""
	}

	return TranslatePath(filepath.Dir(p.v.ConfigFileUsed()), path)
}

func (p *Provider) TranslatePath(path string) string {
	if path == "" {
		return ""
	}

	return TranslatePath(filepath.Dir(p.v.ConfigFileUsed()), path)
}

func (p *Provider) GetString(key string) string {
	return p.v.GetString(key)
}

func (p *Provider) ConfigFileUsed() string {
	return p.v.ConfigFileUsed()
}

func (p *Provider) load() error {
	p.v = viper.New()
	err := p.initViper(p.v, CmdRoot)
	if err != nil {
		return err
	}

	err = p.v.ReadInConfig() // Find and read the config file
	if err != nil {          // Handle errors reading the config file
		// The version of Viper we use claims the config type isn't supported when in fact the file hasn't been found
		// Display a more helpful message to avoid confusing the user.
		if strings.Contains(fmt.Sprint(err), "Unsupported Config Type") {
			return errors.Errorf("Could not find config file. "+
				"Please make sure that FSCNODE_CFG_PATH is set to a path "+
				"which contains %s.yaml", CmdRoot)
		} else {
			return errors.WithMessagef(err, "error when reading %s config file", CmdRoot)
		}
	}

	if err := p.substituteEnv(); err != nil {
		return err
	}

	logging.Init(logging.Config{
		Format:  p.v.GetString("logging.format"),
		Writer:  logOutput,
		LogSpec: p.v.GetString("logging.spec"),
	})

	return nil
}

// Manually override keys if the respective environment variable is set, because viper doesn't do
// that for UnmarshalKey values (see https://github.com/spf13/viper/pull/1699).
// Example: CORE_LOGGING_FORMAT sets logging.format.
func (p *Provider) substituteEnv() error {
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, strings.ToUpper(CmdRoot)+"_") {
			continue
		}

		env := strings.Split(e, "=")
		val := env[1]
		if len(val) == 0 {
			continue
		}
		key, val := env[0], strings.Join(env[1:], "=")

		noprefix := strings.TrimLeft(key, strings.ToUpper(CmdRoot)+"_")
		key = strings.ToLower(strings.ReplaceAll(noprefix, "_", "."))

		// nested key
		keys := strings.Split(key, ".")
		parent := strings.Join(keys[:len(keys)-1], ".")
		if !p.v.IsSet(parent) {
			fmt.Println("applying " + env[0] + " - parent not found in core.yaml: " + parent)
			p.v.Set(key, val)
			continue
		}

		k := p.v.GetStringMap(key)
		if len(k) > 0 {
			fmt.Println("-- skipping " + env[0] + ": cannot override maps")
			continue
		}

		root := p.v.GetStringMap(keys[0])
		if err := setDeepValue(root, keys, val); err != nil {
			return errors.Wrap(err, "error when substituting")
		}
		p.v.Set(keys[0], root)
		fmt.Println("applying " + env[0])
	}
	return nil
}

// Function to set the value at the deepest level
func setDeepValue(m map[string]any, keys []string, value any) error {
	// key = root but we don't have the map by reference
	if len(keys) < 2 {
		return errors.New("can't set root key")
	}

	current := m
	// traverse to the last map
	for i := 1; i < len(keys)-1; i++ {
		key := keys[i]
		nextMap, ok := current[key].(map[string]any)
		if !ok {
			return errors.New("expected map at key " + key)
		}
		current = nextMap
	}
	lastKey := keys[len(keys)-1]
	current[lastKey] = value

	return nil
}

// ----------------------------------------------------------------------------------
// InitViper()
// ----------------------------------------------------------------------------------
// Performs basic initialization of our viper-based configuration layer.
// Primary thrust is to establish the paths that should be consulted to find
// the configuration we need.  If v == nil, we will initialize the global
// Viper instance
// ----------------------------------------------------------------------------------
func (p *Provider) initViper(v *viper.Viper, configName string) error {
	if len(p.confPath) != 0 {
		AddConfigPath(v, p.confPath)
	}

	var altPath = os.Getenv("FSCNODE_CFG_PATH")
	if altPath != "" {
		// If the user has overridden the path with an envvar, its the only path
		// we will consider

		if !dirExists(altPath) {
			return errors.Errorf("FSCNODE_CFG_PATH %s does not exist", altPath)
		}

		AddConfigPath(v, altPath)
	} else {
		// If we get here, we should use the default paths in priority order:
		//
		// *) CWD
		// *) /etc/hyperledger/fsc

		// CWD
		AddConfigPath(v, "./")

		// And finally, the official path
		if dirExists(OfficialPath) {
			AddConfigPath(v, OfficialPath)
		}
	}

	// Now set the configuration file.
	if v != nil {
		v.SetConfigName(configName)
	} else {
		viper.SetConfigName(configName)
	}

	return nil
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func AddConfigPath(v *viper.Viper, p string) {
	if v != nil {
		v.AddConfigPath(p)
	} else {
		viper.AddConfigPath(p)
	}
}

func TranslatePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}

	return filepath.Join(base, p)
}
