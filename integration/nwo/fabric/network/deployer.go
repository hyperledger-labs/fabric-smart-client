/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"fmt"
	"path/filepath"

	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/ccaas"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

// deployLegacy packages the chaincode's Go source and lets the peer build it
// in a ccenv-derived container.
func (n *Network) deployLegacy(chaincode *topology.ChannelChaincode) {
	orderer := n.Orderer("orderer")
	peers := n.PeersForChaincodeByName(chaincode.Peers)

	if len(chaincode.Chaincode.PackageFile) == 0 {
		chaincode.Chaincode.PackageFile = filepath.Join(
			n.Context.RootDir(), n.Prefix,
			chaincode.Chaincode.Name+chaincode.Chaincode.Version+".tar.gz")
	}

	PackageAndInstallChaincode(n, &chaincode.Chaincode, peers...)
	ApproveChaincodeForMyOrg(n, chaincode.Channel, orderer, &chaincode.Chaincode, peers...)
	CheckCommitReadinessUntilReady(n, chaincode.Channel, &chaincode.Chaincode, n.PeerOrgsByPeers(peers), peers...)
	CommitChaincode(n, chaincode.Channel, orderer, &chaincode.Chaincode, peers[0], peers...)
	for _, peer := range peers {
		QueryInstalledReferences(n,
			chaincode.Channel, chaincode.Chaincode.Label, chaincode.Chaincode.PackageID,
			peer,
			[]string{chaincode.Chaincode.Name, chaincode.Chaincode.Version})
	}
	if chaincode.Chaincode.InitRequired {
		InitChaincode(n, chaincode.Channel, orderer, &chaincode.Chaincode, peers...)
	}
	n.topology.AddChaincode(chaincode)
}

// orgGroup is one organization's slice of a CCaaS deployment: the peers that
// share a chaincode server, and the MSP ID that server declares.
type orgGroup struct {
	Org   string
	MSPID string
	Peers []*topology.Peer
}

// orgPeerGroups groups peers by organization, preserving the order in which the
// organizations first appear. The order matters: ports, package files, and
// container names are derived from it, and stable ordering keeps runs
// reproducible. Peers of one org share a server because CORE_PEER_LOCALMSPID is
// an org-level fact, not a peer-level one.
func orgPeerGroups(peers []*topology.Peer, mspidOf func(org string) string) []orgGroup {
	var groups []orgGroup
	index := map[string]int{}
	for _, p := range peers {
		i, ok := index[p.Organization]
		if !ok {
			i = len(groups)
			index[p.Organization] = i
			groups = append(groups, orgGroup{Org: p.Organization, MSPID: mspidOf(p.Organization)})
		}
		groups[i].Peers = append(groups[i].Peers, p)
	}
	return groups
}

// deployCCaaS runs one chaincode server container per organization and hands
// each org's peers a ccaas package pointing at its own server.
func (n *Network) deployCCaaS(chaincode *topology.ChannelChaincode) {
	cc := &chaincode.Chaincode

	gomega.Expect(ccaas.EnsureImagePresent(cc.Image)).To(gomega.Succeed())

	// One server per organization. Chaincode that calls shim.GetMSPID reads
	// CORE_PEER_LOCALMSPID, an org-level fact, so peers of different orgs
	// cannot share a server.
	peers := n.PeersForChaincodeByName(chaincode.Peers)
	groups := orgPeerGroups(peers, func(org string) string { return n.Organization(org).MSPID })
	gomega.Expect(groups).NotTo(gomega.BeEmpty(), "chaincode [%s] has no peers", cc.Name)

	orderer := n.Orderer("orderer")
	orgCCs := make([]*topology.Chaincode, 0, len(groups))
	for _, g := range groups {
		port := n.Context.ReservePort()
		address := fmt.Sprintf("127.0.0.1:%d", port)

		// Each org's connection.json holds a different address, hence a
		// different package id; fabric treats the package id as org-local.
		pkgFile := filepath.Join(n.Context.RootDir(), n.Prefix, "ccaas",
			fmt.Sprintf("%s%s-%s.tar.gz", cc.Name, cc.Version, g.Org))
		gomega.Expect(ccaas.BuildPackage(pkgFile, cc.Label,
			ccaas.Connection{Address: address, DialTimeout: "10s"})).To(gomega.Succeed())

		orgCC := *cc
		orgCC.PackageFile = pkgFile
		orgCC.SetPackageIDFromPackageFile()

		shortID := orgCC.PackageID
		if len(shortID) > 8 {
			shortID = shortID[len(shortID)-8:]
		}
		gomega.Expect(n.containerManager().Start(ccaas.ContainerSpec{
			Name:          fmt.Sprintf("%s-cc-%s-%s-%s", n.NetworkID, cc.Label, g.Org, shortID),
			Image:         cc.Image,
			NetworkID:     n.NetworkID,
			Port:          port,
			CCID:          orgCC.PackageID,
			ServerAddress: fmt.Sprintf("0.0.0.0:%d", port),
			MSPID:         g.MSPID,
		})).To(gomega.Succeed())

		InstallChaincode(n, &orgCC, g.Peers...)
		ApproveChaincodeForMyOrg(n, chaincode.Channel, orderer, &orgCC, g.Peers...)
		orgCCs = append(orgCCs, &orgCC)
	}

	// Commit is network-wide and carries no package id.
	CheckCommitReadinessUntilReady(n, chaincode.Channel, cc, n.PeerOrgsByPeers(peers), peers...)
	CommitChaincode(n, chaincode.Channel, orderer, cc, peers[0], peers...)
	for i, g := range groups {
		for _, peer := range g.Peers {
			QueryInstalledReferences(n, chaincode.Channel, cc.Label, orgCCs[i].PackageID,
				peer, []string{cc.Name, cc.Version})
		}
	}
	if cc.InitRequired {
		InitChaincode(n, chaincode.Channel, orderer, cc, peers...)
	}

	// No package id is written back to cc: there is no single value under
	// CCaaS, and nothing reads the field after deploy.
	n.topology.AddChaincode(chaincode)
}
