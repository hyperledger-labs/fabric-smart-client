/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

type configtx struct {
	Organizations []struct {
		Name             string   `yaml:"Name"`
		OrdererEndpoints []string `yaml:"OrdererEndpoints"`
	} `yaml:"Organizations"`
	Profiles map[string]struct {
		Orderer struct {
			ConsenterMapping []consenter `yaml:"ConsenterMapping"`
		} `yaml:"Orderer"`
	} `yaml:"Profiles"`
}

type consenter struct {
	ID       int    `yaml:"ID"`
	Host     string `yaml:"Host"`
	Port     int    `yaml:"Port"`
	MSPID    string `yaml:"MSPID"`
	Identity string `yaml:"Identity"`
}

const (
	testOrdererCount = 4
	// base values chosen so that every orderer ends up with a distinct host and
	// distinct ports: a template that silently uses the wrong orderer therefore
	// produces a different address instead of an accidentally matching one.
	testListenPortBase  = 21100
	testClusterPortBase = 21200
)

func testOrdererHost(i int) string { return fmt.Sprintf("10.0.0.%d", i+1) }

func testListenPort(i int) uint16 { return uint16(testListenPortBase + i) }

func testClusterPort(i int) uint16 { return uint16(testClusterPortBase + i) }

// generateConfigTx renders configtx.yaml for a network of testOrdererCount
// orderers, each with its own host and its own ports, and returns the parsed
// result. profileOrderers controls the order in which the profile references
// them, which need not match the network-wide order.
func generateConfigTx(t *testing.T, consensusType string, profileOrderers []string) configtx {
	t.Helper()
	gomega.RegisterTestingT(t)

	orderers := make([]*topology.Orderer, 0, testOrdererCount)
	for i := range testOrdererCount {
		orderers = append(orderers, &topology.Orderer{
			Name:         fmt.Sprintf("orderer%d", i+1),
			Organization: "OrdererOrg",
			Id:           i + 1,
		})
	}

	topo := &topology.Topology{
		TopologyName: "default",
		TopologyType: "fabric",
		Driver:       "generic",
		Default:      true,
		Consensus:    &topology.Consensus{Type: consensusType},
		Orderers:     orderers,
		Organizations: []*topology.Organization{{
			ID:     "OrdererOrg",
			Name:   "OrdererOrg",
			MSPID:  "OrdererMSP",
			Domain: "example.com",
		}},
		Profiles: []*topology.Profile{{
			Name:     "OrgsChannel",
			Orderers: profileOrderers,
		}},
	}

	n := New(nwocontext.New(t.TempDir(), 0, nil), topo, nil, nil, "test-network")
	n.Templates = &topology.Templates{}

	for i, o := range orderers {
		n.Context.SetPortsByOrdererID(n.Prefix, o.ID(), api.Ports{
			ListenPort:  testListenPort(i),
			ClusterPort: testClusterPort(i),
		})
		n.Context.SetHostByOrdererID(n.Prefix, o.ID(), testOrdererHost(i))
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(n.ConfigTxConfigPath()), 0o755))
	n.GenerateConfigTxConfig()

	raw, err := os.ReadFile(n.ConfigTxConfigPath())
	require.NoError(t, err)

	var conf configtx
	require.NoError(t, yaml.Unmarshal(raw, &conf), "generated configtx.yaml is not valid yaml:\n%s", raw)

	return conf
}

// allOrderers returns the network-wide orderer names in order.
func allOrderers() []string {
	names := make([]string, 0, testOrdererCount)
	for i := range testOrdererCount {
		names = append(names, fmt.Sprintf("orderer%d", i+1))
	}
	return names
}

// requireSharedEndpoint asserts that every logical orderer resolves to the
// endpoint of orderer1 -- the only orderer the fabric-x committer container
// actually binds -- while keeping four distinct consenter identities.
func requireSharedEndpoint(t *testing.T, conf configtx) {
	t.Helper()

	wantHost := testOrdererHost(0)
	wantPort := testListenPort(0)
	wantEndpoint := net.JoinHostPort(wantHost, strconv.Itoa(int(wantPort)))

	require.Len(t, conf.Organizations, 1)
	require.Len(t, conf.Organizations[0].OrdererEndpoints, testOrdererCount)
	for _, endpoint := range conf.Organizations[0].OrdererEndpoints {
		require.Equal(t, wantEndpoint, endpoint, "every orderer endpoint must point at orderer1")
	}

	profile, ok := conf.Profiles["OrgsChannel"]
	require.True(t, ok)
	require.Len(t, profile.Orderer.ConsenterMapping, testOrdererCount)

	identities := make(map[string]struct{}, testOrdererCount)
	ids := make(map[int]struct{}, testOrdererCount)
	for _, c := range profile.Orderer.ConsenterMapping {
		require.Equal(t, wantHost, c.Host, "every consenter must point at orderer1's host")
		require.Equal(t, int(wantPort), c.Port, "every consenter must point at orderer1's port")
		require.Equal(t, "OrdererMSP", c.MSPID)
		identities[c.Identity] = struct{}{}
		ids[c.ID] = struct{}{}
	}
	require.Len(t, identities, testOrdererCount, "each consenter must keep its own signing identity")
	require.Len(t, ids, testOrdererCount, "each consenter must keep its own id")
}

// TestConfigTxArmaMultiOrdererSharedPort checks that an arma network maps all
// logical orderers onto the single endpoint exposed by the committer container.
func TestConfigTxArmaMultiOrdererSharedPort(t *testing.T) { //nolint:paralleltest // gomega.RegisterTestingT is process-wide
	requireSharedEndpoint(t, generateConfigTx(t, "arma", allOrderers()))
}

// TestConfigTxArmaProfileOrdererOrderIndependent checks that the shared
// endpoint is the network's first orderer even when the profile lists the
// orderers in a different order: the container binds the network's first
// orderer regardless of what any profile says.
func TestConfigTxArmaProfileOrdererOrderIndependent(t *testing.T) { //nolint:paralleltest // gomega.RegisterTestingT is process-wide
	requireSharedEndpoint(t, generateConfigTx(t, "arma", []string{"orderer2", "orderer3", "orderer4", "orderer1"}))
}

// TestConfigTxBFTConsenterMapping checks that a plain BFT network keeps one
// consenter per orderer, each with its own host and cluster port.
func TestConfigTxBFTConsenterMapping(t *testing.T) { //nolint:paralleltest // gomega.RegisterTestingT is process-wide
	conf := generateConfigTx(t, "BFT", allOrderers())

	profile, ok := conf.Profiles["OrgsChannel"]
	require.True(t, ok)
	require.Len(t, profile.Orderer.ConsenterMapping, testOrdererCount)

	for i, c := range profile.Orderer.ConsenterMapping {
		require.Equal(t, i+1, c.ID)
		require.Equal(t, testOrdererHost(i), c.Host, "consenter must carry its own host")
		require.Equal(t, int(testClusterPort(i)), c.Port, "consenter must carry its own cluster port")
		require.Equal(t, "OrdererMSP", c.MSPID)
		require.Contains(t, c.Identity, fmt.Sprintf("orderer%d.example.com-cert.pem", i+1))
	}
}
