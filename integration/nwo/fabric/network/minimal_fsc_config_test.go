/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"strings"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
)

func newTestFSCPeer(t *testing.T) (*Network, *topology.Peer) {
	t.Helper()
	gomega.RegisterTestingT(t)

	topo := &topology.Topology{
		TopologyName: "default",
		TopologyType: "fabric",
		Driver:       "generic",
		Default:      true,
		Consortiums:  []*topology.Consortium{{Name: "SampleConsortium"}},
	}
	topo.AppendOrganization(&topology.Organization{
		ID:     "Org1",
		Name:   "Org1",
		MSPID:  "Org1MSP",
		Domain: "org1.example.com",
	})

	n := New(nwocontext.New(t.TempDir(), 0, nil), topo, nil, nil, "test-network")
	n.Templates = &topology.Templates{}

	fscNode := node.NewNode("fsc-node-1")
	peer := &topology.Peer{
		Name:            "fsc-node-1",
		Organization:    "Org1",
		Type:            topology.FSCPeer,
		DefaultNetwork:  true,
		DefaultIdentity: "fsc-node-1",
		FSCNode:         fscNode,
		Identities: []*topology.PeerIdentity{
			{
				ID:      "fsc-node-1",
				MSPType: "bccsp",
				MSPID:   "Org1MSP",
				Org:     "Org1",
				Opts:    BCCSPOpts("SW"),
				Default: true,
			},
		},
	}
	n.Peers = append(n.Peers, peer)

	return n, peer
}

func TestGenerateCoreConfig_MinimalFSCFabricConfig(t *testing.T) { //nolint:paralleltest
	n, peer := newTestFSCPeer(t)
	n.topology.MinimalFSCFabricConfig = true

	n.GenerateCoreConfig(peer)

	uniqueName := peer.FSCNode.ReplicaUniqueNames()[0]
	extensions := n.Context.ExtensionsByPeerID(uniqueName)
	require.Contains(t, extensions, api.FabricExtension)

	fragment := extensions[api.FabricExtension][0]
	assert.Equal(t, "fabric:\n  enabled: true", strings.TrimSpace(fragment))
	assert.NotContains(t, fragment, "peers:")
	assert.NotContains(t, fragment, "channels:")
	assert.NotContains(t, fragment, "Org1")
}

func TestGenerateCoreConfig_DefaultFSCFabricConfig(t *testing.T) { //nolint:paralleltest
	n, peer := newTestFSCPeer(t)
	// MinimalFSCFabricConfig left false (default) - full extension should be rendered.

	n.GenerateCoreConfig(peer)

	uniqueName := peer.FSCNode.ReplicaUniqueNames()[0]
	extensions := n.Context.ExtensionsByPeerID(uniqueName)
	require.Contains(t, extensions, api.FabricExtension)

	fragment := extensions[api.FabricExtension][0]
	assert.Contains(t, fragment, "fabric:")
	assert.Contains(t, fragment, "enabled: true")
	assert.Contains(t, fragment, "peers:")
}
