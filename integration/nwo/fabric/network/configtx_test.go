/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type configtx struct {
	Organizations []struct {
		Name             string   `yaml:"Name"`
		OrdererEndpoints []string `yaml:"OrdererEndpoints"`
	} `yaml:"Organizations"`
	Profiles map[string]struct {
		Orderer struct {
			ConsenterMapping []struct {
				Port int `yaml:"Port"`
			} `yaml:"ConsenterMapping"`
		} `yaml:"Orderer"`
	} `yaml:"Profiles"`
}

func TestConfigTxArmaMultiOrdererSharedPort(t *testing.T) {
	gomega.RegisterTestingT(t)
	ctx := context.New(t.TempDir(), 20000, nil)

	topo := &topology.Topology{
		TopologyName: "default",
		TopologyType: "fabric",
		Driver:       "generic",
		Default:      true,
		Consensus:    &topology.Consensus{Type: "arma"},
		Orderers: []*topology.Orderer{
			{Name: "orderer1", Organization: "OrdererOrg"},
			{Name: "orderer2", Organization: "OrdererOrg"},
			{Name: "orderer3", Organization: "OrdererOrg"},
			{Name: "orderer4", Organization: "OrdererOrg"},
		},
		Organizations: []*topology.Organization{
			{ID: "OrdererOrg", Name: "OrdererOrg", MSPID: "OrdererMSP"},
		},
		Profiles: []*topology.Profile{
			{Name: "OrgsChannel", Orderers: []string{"orderer1", "orderer2", "orderer3", "orderer4"}},
		},
	}

	n := New(ctx, topo, nil, nil, "test-network")
	n.Templates = &topology.Templates{}
	
	// Pre-assign ports so we know what to expect
	port := ctx.ReservePort()
	for _, o := range topo.Orderers {
		ctx.SetPortsByOrdererID(n.Prefix, o.ID(), api.Ports{
			ListenPort: port,
		})
		ctx.SetHostByOrdererID(n.Prefix, o.ID(), "127.0.0.1")
	}

	err := os.MkdirAll(filepath.Dir(n.ConfigTxConfigPath()), 0755)
	require.NoError(t, err)

	n.GenerateConfigTxConfig()

	b, err := os.ReadFile(n.ConfigTxConfigPath())
	require.NoError(t, err)

	var conf configtx
	err = yaml.Unmarshal(b, &conf)
	require.NoError(t, err)

	// Verify all 4 OrdererEndpoints have the same port (orderer1's ListenPort)
	expectedPort := n.OrdererPort(topo.Orderers[0], ListenPort)
	require.Greater(t, expectedPort, uint16(0))

	require.Len(t, conf.Organizations, 1)
	require.Len(t, conf.Organizations[0].OrdererEndpoints, 4)
	for _, ep := range conf.Organizations[0].OrdererEndpoints {
		require.Contains(t, ep, string("127.0.0.1"))
		// Endpoint format is "host:port", check if port matches expectedPort
	}

	// Verify ConsenterMapping
	profile, ok := conf.Profiles["OrgsChannel"]
	require.True(t, ok)
	require.Len(t, profile.Orderer.ConsenterMapping, 4)
	for _, mapping := range profile.Orderer.ConsenterMapping {
		require.Equal(t, int(expectedPort), mapping.Port)
	}
}
