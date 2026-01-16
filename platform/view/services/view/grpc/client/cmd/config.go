/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"gopkg.in/yaml.v2"
)

type SignerConfig struct {
	MSPID        string
	IdentityPath string
	KeyPath      string
}

// TLSClientConfig defines configuration of a Client
type TLSClientConfig struct {
	Enabled            bool   `yaml:"enabled"`
	RootCACertPath     string `yaml:"rootCACertPath,omitempty"`
	ClientAuthRequired bool   `yaml:"clientAuthRequired,omitempty"`
	ClientCertPath     string `yaml:"clientCertPath,omitempty"`
	ClientKeyPath      string `yaml:"clientKeyPath,omitempty"`
}

// Config aggregates configuration of TLS and signing
type Config struct {
	Version      int
	Address      string
	TLSConfig    TLSClientConfig
	SignerConfig SignerConfig
}

// ConfigFromFile loads the given file and converts it to a Config
func ConfigFromFile(file string) (Config, error) {
	configData, err := os.ReadFile(file)
	if err != nil {
		return Config{}, errors.WithStack(err)
	}
	config := Config{}

	if err := yaml.Unmarshal(configData, &config); err != nil {
		return Config{}, errors.Errorf("error unmarshalling YAML file %s: %s", file, err)
	}

	return config, validateConfig(config)
}

// ToFile writes the config into a file
func (c Config) ToFile(file string) error {
	if err := validateConfig(c); err != nil {
		return errors.Wrap(err, "config isn't valid")
	}
	b, _ := yaml.Marshal(c)
	if err := os.WriteFile(file, b, 0o600); err != nil {
		return errors.Errorf("failed writing file %s: %v", file, err)
	}
	return nil
}

func validateConfig(conf Config) error {
	nonEmptyStrings := []string{
		conf.SignerConfig.IdentityPath,
		conf.SignerConfig.KeyPath,
	}

	for _, s := range nonEmptyStrings {
		if s == "" {
			return errors.New("empty string that is mandatory")
		}
	}
	return nil
}

func ValidateTLSConfig(config TLSClientConfig) error {
	isEmpty := func(val string) bool {
		return len(val) == 0
	}

	if !config.Enabled {
		return nil
	}

	if isEmpty(config.RootCACertPath) {
		return errors.New("rootCACertPath not set")
	}

	if !config.ClientAuthRequired {
		return nil
	}

	if isEmpty(config.ClientKeyPath) {
		return errors.New("clientKeyPath not set")
	}

	if isEmpty(config.ClientCertPath) {
		return errors.New("ClientCertPath not set")
	}

	return nil
}
