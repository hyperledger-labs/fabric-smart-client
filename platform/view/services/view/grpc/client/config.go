/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// MSPInfo models the MSP information.
type MSPInfo struct {
	MSPConfigPath string
	MSPID         string
	MSPType       string
}

// Config aggregates configuration of the view service client.
type Config struct {
	ID               string
	ConnectionConfig *grpc.ConnectionConfig
}

// ToJSon returns the JSON representation of the config.
func (config *Config) ToJSon() ([]byte, error) {
	return json.Marshal(config)
}

// Configs is a list of Config.
type Configs []Config

// ToJSon returns the JSON representation of the list of configs.
func (configs *Configs) ToJSon() ([]byte, error) {
	return json.MarshalIndent(configs, "", " ")
}

// FromJSON returns a list of Config from the given JSON representation.
func FromJSON(raw []byte) (Configs, error) {
	configs := &Configs{}
	err := json.Unmarshal(raw, configs)
	if err != nil {
		return nil, err
	}
	return *configs, nil
}

// ValidateClientConfig validates the given client config.
func ValidateClientConfig(config Config) error {
	if config.ConnectionConfig.Address == "" {
		return errors.New("missing fsc peer address")
	}
	if config.ConnectionConfig.TLSEnabled && (config.ConnectionConfig.TLSRootCertFile == "" || len(config.ConnectionConfig.TLSRootCertBytes) == 0) {
		return errors.New("missing fsc peer TLSRootCertFile")
	}

	return nil
}
