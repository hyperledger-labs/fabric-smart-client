/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	fabricx_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
)

type mockOrdererConfig struct {
	ConsenterMSPIdentities []struct {
		MSPID  string `yaml:"msp-id"`
		MSPDir string `yaml:"msp-dir"`
	} `yaml:"consenter-msp-identities"`
}

// newOrdererNetwork builds a network whose orderer organization deliberately
// uses a non-default MSP id and domain, so that any hardcoded "OrdererMSP" or
// "example.com" shows up as a wrong path rather than an accidental match.
func newOrdererNetwork(t *testing.T) *fabricx_network.Network {
	t.Helper()

	topo := &topology.Topology{
		TopologyName: "default",
		TopologyType: "fabricx",
		Driver:       "fabricx",
		Organizations: []*topology.Organization{{
			ID:     "OrdererOrg",
			Name:   "OrdererOrg",
			MSPID:  "CustomOrdererMSP",
			Domain: "orderers.example.org",
		}},
		Orderers: []*topology.Orderer{
			{Name: "orderer1", Organization: "OrdererOrg", Id: 1},
			{Name: "orderer2", Organization: "OrdererOrg", Id: 2},
		},
	}

	return fabricx_network.New(
		nwocontext.New(t.TempDir(), 0, nil), topo, nil, nil, "test-network", "Org1", "SC",
	)
}

func TestGenerateMockOrdererConfigFile(t *testing.T) {
	t.Parallel()

	n := newOrdererNetwork(t)

	configPath := filepath.Join(t.TempDir(), "mock-orderer.yaml")
	require.NoError(t, generateMockOrdererConfigFile(configPath, ordererConsenters(n)))

	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var conf mockOrdererConfig
	require.NoError(t, yaml.Unmarshal(raw, &conf), "generated mock-orderer.yaml is not valid yaml:\n%s", raw)

	require.Equal(t, []struct {
		MSPID  string `yaml:"msp-id"`
		MSPDir string `yaml:"msp-dir"`
	}{
		{
			MSPID:  "CustomOrdererMSP",
			MSPDir: "/root/artifacts/crypto/ordererOrganizations/orderers.example.org/orderers/orderer1.orderers.example.org/msp",
		},
		{
			MSPID:  "CustomOrdererMSP",
			MSPDir: "/root/artifacts/crypto/ordererOrganizations/orderers.example.org/orderers/orderer2.orderers.example.org/msp",
		},
	}, conf.ConsenterMSPIdentities)
}
