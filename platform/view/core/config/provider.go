/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/viper"

	viperutil "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config/viper"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const (
	CmdRoot = "core"
)

const OfficialPath = "/etc/hyperledger-labs/fabric-smart-client-node"

var (
	logger    = flogging.MustGetLogger("view-sdk.config")
	logOutput = os.Stderr
)

type provider struct {
	confPath string
	v        *viper.Viper
}

func NewProvider(confPath string) (*provider, error) {
	p := &provider{confPath: confPath}
	if err := p.load(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *provider) GetDuration(key string) time.Duration {
	return p.v.GetDuration(key)
}

func (p *provider) GetBool(key string) bool {
	logger.Debugf("Getting bool %s:%v", key, p.v.Get(key))
	return p.v.GetBool(key)
}

func (p *provider) GetStringSlice(key string) []string {
	return p.v.GetStringSlice(key)
}

func (p *provider) AddDecodeHook(f driver.DecodeHookFuncType) error {
	return nil
}

func (p *provider) UnmarshalKey(key string, rawVal interface{}) error {
	return viperutil.EnhancedExactUnmarshal(p.v, key, rawVal)
}

func (p *provider) IsSet(key string) bool {
	return p.v.IsSet(key)
}

func (p *provider) GetPath(key string) string {
	path := p.v.GetString(key)
	if path == "" {
		return ""
	}

	return TranslatePath(filepath.Dir(p.v.ConfigFileUsed()), path)
}

func (p *provider) TranslatePath(path string) string {
	if path == "" {
		return ""
	}

	return TranslatePath(filepath.Dir(p.v.ConfigFileUsed()), path)
}

func (p *provider) GetString(key string) string {
	return p.v.GetString(key)
}

func (p *provider) ConfigFileUsed() string {
	return p.v.ConfigFileUsed()
}

func (p *provider) load() error {
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

	// read in the legacy logging level settings and, if set,
	// notify users of the FSCNODE_LOGGING_SPEC env variable
	var loggingLevel string
	if p.v.GetString("logging_level") != "" {
		loggingLevel = p.v.GetString("logging_level")
	} else {
		loggingLevel = p.v.GetString("logging.level")
	}
	if loggingLevel != "" {
		logger.Warning("CORE_LOGGING_LEVEL is no longer supported, please use the FSCNODE_LOGGING_SPEC environment variable")
	}

	loggingSpec := os.Getenv("FSCNODE_LOGGING_SPEC")
	loggingFormat := os.Getenv("FSCNODE_LOGGING_FORMAT")

	if len(loggingSpec) == 0 {
		loggingSpec = p.v.GetString("logging.spec")
	}

	flogging.Init(flogging.Config{
		Format:  loggingFormat,
		Writer:  logOutput,
		LogSpec: loggingSpec,
	})

	return nil
}

//----------------------------------------------------------------------------------
// InitViper()
//----------------------------------------------------------------------------------
// Performs basic initialization of our viper-based configuration layer.
// Primary thrust is to establish the paths that should be consulted to find
// the configuration we need.  If v == nil, we will initialize the global
// Viper instance
//----------------------------------------------------------------------------------
func (p *provider) initViper(v *viper.Viper, configName string) error {
	v.SetEnvPrefix(CmdRoot)
	v.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	v.SetEnvKeyReplacer(replacer)

	if len(p.confPath) != 0 {
		AddConfigPath(v, p.confPath)
	}

	var altPath = os.Getenv("FSCNODE_CFG_PATH")
	if altPath != "" {
		// If the user has overridden the path with an envvar, its the only path
		// we will consider

		if !dirExists(altPath) {
			return fmt.Errorf("FSCNODE_CFG_PATH %s does not exist", altPath)
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
