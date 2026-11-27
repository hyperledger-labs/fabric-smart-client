/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evm

import (
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Extension is the name GenerateArtifacts renders each FSC node's "evm" core.yaml section under.
const Extension api.ExtensionName = "EVMExtension"

// NodeConfig is what RenderExtension turns into one FSC node's evm extension.
type NodeConfig struct {
	Endpoint      string
	ChainID       int64
	Contract      client.Address
	KeyPath       string
	GasMultiplier float64
	PollInterval  time.Duration
	Timeout       time.Duration
}

type extensionGas struct {
	Multiplier float64 `yaml:"multiplier"`
}

type extensionFinality struct {
	PollInterval string `yaml:"pollInterval"`
	Timeout      string `yaml:"timeout"`
}

type extensionEVM struct {
	Endpoint string            `yaml:"endpoint"`
	ChainID  int64             `yaml:"chainID"`
	Contract string            `yaml:"contract"`
	Key      string            `yaml:"key"`
	Gas      extensionGas      `yaml:"gas"`
	Finality extensionFinality `yaml:"finality"`
}

type extensionDoc struct {
	EVM extensionEVM `yaml:"evm"`
}

// RenderExtension renders c as the YAML a node's core.yaml merges under "evm": endpoint, chain ID,
// the deployed contract's address, the node's key path, the gas multiplier and the finality
// poll/timeout durations (rendered as Go duration strings, e.g. "1s").
func RenderExtension(c NodeConfig) (string, error) {
	doc := extensionDoc{EVM: extensionEVM{
		Endpoint: c.Endpoint,
		ChainID:  c.ChainID,
		Contract: c.Contract.Hex(),
		Key:      c.KeyPath,
		Gas:      extensionGas{Multiplier: c.GasMultiplier},
		Finality: extensionFinality{
			PollInterval: c.PollInterval.String(),
			Timeout:      c.Timeout.String(),
		},
	}}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", errors.Wrap(err, "evm nwo: failed to render the node extension")
	}

	return string(out), nil
}
