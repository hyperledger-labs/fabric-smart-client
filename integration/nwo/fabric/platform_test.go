/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"os"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

// TestMain wires Gomega's fail handler to panic, the same way integration.go
// does for Ginkgo specs, so the gomega.Expect guards under test raise a
// catchable panic instead of aborting the process outside of Ginkgo.
func TestMain(m *testing.M) {
	gomega.RegisterFailHandler(func(message string, _ ...int) { panic(message) })
	os.Exit(m.Run())
}

func TestUserByOrg(t *testing.T) {
	t.Parallel()
	p := &Platform{Network: &network.Network{
		Context:       nwocontext.New("", 0, nil),
		Organizations: []*topology.Organization{{Name: "org1", Domain: "org1.example.com"}},
		Peers:         []*topology.Peer{{Name: "peer0", Organization: "org1", Type: topology.FabricPeer}},
	}}

	user := p.UserByOrg("org1", "User1")
	require.Equal(t, "User1@org1.example.com", user.Name)
}

func TestUserByOrg_NoPeersInOrg(t *testing.T) {
	t.Parallel()
	p := &Platform{Network: &network.Network{}}
	require.Panics(t, func() { p.UserByOrg("empty-org", "User1") })
}

func TestUsersByOrg(t *testing.T) {
	t.Parallel()
	p := &Platform{Network: &network.Network{
		Context: nwocontext.New("", 0, nil),
		Organizations: []*topology.Organization{{
			Name:      "org1",
			Domain:    "org1.example.com",
			UserSpecs: []topology.UserSpec{{Name: "User1"}, {Name: "User2"}},
		}},
		Peers: []*topology.Peer{{Name: "peer0", Organization: "org1", Type: topology.FabricPeer}},
	}}

	users := p.UsersByOrg("org1")
	require.Len(t, users, 2)
	require.Equal(t, "User1@org1.example.com", users[0].Name)
	require.Equal(t, "User2@org1.example.com", users[1].Name)
}

func TestUsersByOrg_NoPeersInOrg(t *testing.T) {
	t.Parallel()
	p := &Platform{Network: &network.Network{
		Organizations: []*topology.Organization{{Name: "org1", UserSpecs: []topology.UserSpec{{Name: "User1"}}}},
	}}
	require.Panics(t, func() { p.UsersByOrg("org1") })
}

func TestInvokeChaincode_NoPeerOrgs(t *testing.T) {
	t.Parallel()
	p := &Platform{Network: &network.Network{}}
	require.Panics(t, func() { p.InvokeChaincode(&topology.ChannelChaincode{}, "method") })
}

func TestConnectionProfile_NoFabricPeersInOrg(t *testing.T) {
	t.Parallel()
	// org1 has a peer, so it is not skipped by PeerOrgs, but the peer is not a
	// FabricPeer, so PeersByOrg(..., includeAll=false) filters it out, leaving
	// the org with zero peers.
	p := &Platform{Network: &network.Network{
		Context:       nwocontext.New("", 0, nil),
		Organizations: []*topology.Organization{{Name: "org1", Domain: "org1.example.com"}},
		Peers:         []*topology.Peer{{Name: "fsc0", Organization: "org1", Type: topology.FSCPeer}},
	}}
	require.Panics(t, func() { p.ConnectionProfile("test", false) })
}
