/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package relay

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type MSPInfo struct {
	MSPConfigPath string
	MSPID         string
	MSPType       string
}

// ClientConfig will be updated after the CR for token client config is merged, where the config data
// will be populated based on a config file.
type ClientConfig struct {
	ID          string
	RelayServer *grpc.ConnectionConfig
}

func (config *ClientConfig) ToJSon() ([]byte, error) {
	return json.Marshal(config)
}

type Configs []ClientConfig

func (configs *Configs) ToJSon() ([]byte, error) {
	return json.MarshalIndent(configs, "", " ")
}

func FromJSON(raw []byte) (Configs, error) {
	configs := &Configs{}
	err := json.Unmarshal(raw, configs)
	if err != nil {
		return nil, err
	}
	return *configs, nil
}

func ValidateClientConfig(config ClientConfig) error {
	if config.RelayServer.Address == "" {
		return errors.New("missing fsc peer address")
	}
	if config.RelayServer.TLSEnabled && (config.RelayServer.TLSRootCertFile == "" || len(config.RelayServer.TLSRootCertBytes) == 0) {
		return errors.New("missing fsc peer TLSRootCertFile")
	}

	return nil
}
