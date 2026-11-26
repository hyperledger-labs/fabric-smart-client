/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	nwocontext "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	nwonode "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node"
)

func orgsByName(p *Platform) map[string]*nwonode.Organization {
	orgs := map[string]*nwonode.Organization{}
	for _, org := range p.Organizations {
		orgs[org.Name] = org
	}
	return orgs
}

func peerOrg(p *Platform, name string) string {
	for _, peer := range p.Peers {
		if peer.Name == name {
			return peer.Organization
		}
	}
	return ""
}

// TestCheckTopology_Organizations checks that WithOrganization places a node in an
// organization of its own and that the resolvers carry that organization's domain.
func TestCheckTopology_Organizations(t *testing.T) { //nolint:paralleltest // the platform asserts through the global gomega
	gomega.RegisterTestingT(t)

	topology := NewTopology()
	topology.AddNodeByName("alice").AddOptions(WithOrganization("org1"))
	topology.AddNodeByName("charlie").AddOptions(WithOrganization("org2"))
	topology.AddNodeByName("bob")
	p := NewPlatform(nwocontext.New(t.TempDir(), 20000, nil), topology, nil)

	orgs := orgsByName(p)
	require.Len(t, orgs, 3)
	require.Contains(t, orgs, "fsc")
	require.Contains(t, orgs, "org1")
	require.Equal(t, "org2MSP", orgs["org2"].MSPID)
	require.Equal(t, "org2.example.com", orgs["org2"].Domain)
	require.Contains(t, orgs["org2"].UserNames, "charlie")
	require.NotContains(t, orgs["fsc"].UserNames, "charlie")

	require.Equal(t, "org1", peerOrg(p, "alice"))
	require.Equal(t, "fsc", peerOrg(p, "bob"))

	p.GenerateResolverMap()
	domains := map[string]string{}
	for _, r := range p.Resolvers {
		domains[r.Name] = r.Domain
	}
	require.Equal(t, map[string]string{
		"alice":   "org1.example.com",
		"charlie": "org2.example.com",
		"bob":     "fsc.example.com",
	}, domains)
}

// TestCheckTopology_DefaultOrganization checks that a topology without WithOrganization
// still puts every node in the single organization fsc.
func TestCheckTopology_DefaultOrganization(t *testing.T) { //nolint:paralleltest // the platform asserts through the global gomega
	gomega.RegisterTestingT(t)

	topology := NewTopology()
	topology.AddNodeByName("alice")
	topology.AddNodeByName("bob")
	p := NewPlatform(nwocontext.New(t.TempDir(), 20000, nil), topology, nil)

	require.Len(t, p.Organizations, 1)
	org := p.Organizations[0]
	require.Equal(t, "fsc", org.Name)
	require.Equal(t, "fscMSP", org.MSPID)
	require.Equal(t, 4, org.Users)
	require.Equal(t, []string{"alice", "bob", "User1", "User2"}, org.UserNames)
}
