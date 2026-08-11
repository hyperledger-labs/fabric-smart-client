/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

func TestIsCCaaS(t *testing.T) { //nolint:paralleltest
	for _, tc := range []struct { //nolint:paralleltest
		name string
		cc   topology.Chaincode
		want bool
	}{
		{"image set", topology.Chaincode{Image: "fsc-cc/base:latest"}, true},
		{"path set", topology.Chaincode{Path: "github.com/acme/cc"}, false},
		{"neither set", topology.Chaincode{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.cc.IsCCaaS())
		})
	}
}

func TestOrgPeerGroups(t *testing.T) { //nolint:paralleltest
	mspidOf := func(org string) string { return org + "MSP" }

	groups := orgPeerGroups([]*topology.Peer{
		{Name: "org1_peer", Organization: "Org1"},
		{Name: "org2_peer", Organization: "Org2"},
		{Name: "org1_peer2", Organization: "Org1"},
	}, mspidOf)

	require.Len(t, groups, 2, "want one group per org")
	require.Equal(t, "Org1", groups[0].Org)
	require.Equal(t, "Org1MSP", groups[0].MSPID)
	require.Equal(t, "Org2", groups[1].Org)
	require.Equal(t, "Org2MSP", groups[1].MSPID)

	require.Len(t, groups[0].Peers, 2, "Org1 group must hold both its peers")
	require.Equal(t, "org1_peer", groups[0].Peers[0].Name)
	require.Equal(t, "org1_peer2", groups[0].Peers[1].Name)
	require.Len(t, groups[1].Peers, 1)
	require.Equal(t, "org2_peer", groups[1].Peers[0].Name)
}

func TestOrgPeerGroupsNoPeers(t *testing.T) { //nolint:paralleltest
	require.Empty(t, orgPeerGroups(nil, func(string) string { return "" }))
}
