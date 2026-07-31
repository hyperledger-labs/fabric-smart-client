/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

func TestSelectDeployer(t *testing.T) {
	ccaas := &fakeDeployer{name: "ccaas"}
	legacy := &fakeDeployer{name: "legacy"}
	n := &Network{ccaasDeployer: ccaas, legacyDeployer: legacy}

	cc := &topology.ChannelChaincode{Chaincode: topology.Chaincode{}}
	if got := n.deployerFor(cc); got != ChaincodeDeployer(ccaas) {
		t.Fatalf("default should be ccaas")
	}

	cc.Chaincode.Deploy = "legacy"
	if got := n.deployerFor(cc); got != ChaincodeDeployer(legacy) {
		t.Fatalf("Deploy=legacy should select legacy")
	}

	cc.Chaincode.Deploy = ""
	t.Setenv("FSC_CHAINCODE_DEPLOY", "legacy")
	if got := n.deployerFor(cc); got != ChaincodeDeployer(legacy) {
		t.Fatalf("env override should force legacy")
	}
}

func TestOrgPeerGroups(t *testing.T) {
	mspidOf := func(org string) string { return org + "MSP" }

	peers := []*topology.Peer{
		{Name: "org1_peer", Organization: "Org1"},
		{Name: "org2_peer", Organization: "Org2"},
		{Name: "org1_peer2", Organization: "Org1"},
	}

	groups := orgPeerGroups(peers, mspidOf)

	if len(groups) != 2 {
		t.Fatalf("want one group per org, got %d: %+v", len(groups), groups)
	}
	if groups[0].Org != "Org1" || groups[0].MSPID != "Org1MSP" {
		t.Errorf("groups[0] = %s/%s, want Org1/Org1MSP", groups[0].Org, groups[0].MSPID)
	}
	if groups[1].Org != "Org2" || groups[1].MSPID != "Org2MSP" {
		t.Errorf("groups[1] = %s/%s, want Org2/Org2MSP", groups[1].Org, groups[1].MSPID)
	}
	if len(groups[0].Peers) != 2 ||
		groups[0].Peers[0].Name != "org1_peer" ||
		groups[0].Peers[1].Name != "org1_peer2" {
		t.Errorf("Org1 group must hold both its peers in order, got %+v", groups[0].Peers)
	}
	if len(groups[1].Peers) != 1 || groups[1].Peers[0].Name != "org2_peer" {
		t.Errorf("Org2 group must hold one peer, got %+v", groups[1].Peers)
	}
}

func TestOrgPeerGroupsSingleOrg(t *testing.T) {
	groups := orgPeerGroups(
		[]*topology.Peer{{Name: "p", Organization: "Org1"}},
		func(string) string { return "Org1MSP" },
	)
	if len(groups) != 1 || groups[0].MSPID != "Org1MSP" || len(groups[0].Peers) != 1 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}

func TestOrgPeerGroupsNoPeers(t *testing.T) {
	if groups := orgPeerGroups(nil, func(string) string { return "" }); len(groups) != 0 {
		t.Fatalf("want no groups for no peers, got %+v", groups)
	}
}

type fakeDeployer struct{ name string }

func (f *fakeDeployer) Deploy(*Network, *topology.ChannelChaincode) {}
func (f *fakeDeployer) Cleanup() error                              { return nil }
