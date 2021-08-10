/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/fabricconfig"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/packager"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

type Connection struct {
	Address     string `json:"address"`
	DialTimeout string `json:"dial_timeout"`
	TlsRequired bool   `json:"tls_required"`
}

// CheckTopologyFPC ensures that the topology contains what is necessary to deploy FPC
func (n *Network) CheckTopologyFPC() {
	if !n.topology.FPC {
		return
	}

	// Define External Builder
	cwd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	index := strings.LastIndex(cwd, "github.com/hyperledger-labs/fabric-smart-client")
	Expect(index).ToNot(BeEquivalentTo(-1))

	path := cwd[:index] + "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/fpc/externalbuilders/chaincode_server"
	n.ExternalBuilders = append(n.ExternalBuilders, fabricconfig.ExternalBuilder{
		Path:                 path,
		Name:                 "fpc-external-launcher",
		PropagateEnvironment: []string{"CORE_PEER_ID", "FABRIC_LOGGING_SPEC"},
	})

	// Reserve ports for Enclave Registry
	for _, org := range n.topology.Organizations {
		n.FPCERCCPort[org.Name] = n.Context.ReservePort()
	}
}

// GenerateArtifactsFPC generates FPC related artifacts
func (n *Network) GenerateArtifactsFPC() {
	// Generate Enclave Registry CC package for each peer that will conn
	Expect(os.MkdirAll(n.ERCCPackagePath(), 0770)).NotTo(HaveOccurred())

	for _, org := range n.PeerOrgs() {
		var p *topology.Peer
		for _, peer := range n.Peers {
			if peer.Organization == org.Name {
				p = peer
				break
			}
		}
		Expect(p).NotTo(BeNil())

		Expect(packager.New().PackageChaincode(
			"ercc",
			"external",
			"ercc_1.0",
			filepath.Join(n.ERCCPackagePath(), fmt.Sprintf("ercc.%s.%s.tgz", p.Name, org.Domain)),
			func(s string, s2 string) (string, []byte) {
				if strings.HasSuffix(s, "connection.json") {
					raw, err := json.MarshalIndent(&Connection{
						Address:     "localhost:" + strconv.Itoa(int(n.FPCERCCPort[org.Name])),
						DialTimeout: "10s",
						TlsRequired: false,
					}, "", " ")
					Expect(err).NotTo(HaveOccurred())
					return filepath.Join(fmt.Sprintf("%s.%s", p.Name, org.Domain), "connection.json"), raw
				}
				return "", nil
			},
		)).ToNot(HaveOccurred())
	}
}

// PostRunFPC executes FPC-related artifacts
func (n *Network) PostRunFPC() {
	if !n.topology.FPC {
		return
	}
}

func (n *Network) DeployFPChaincode(chaincode *topology.ChannelChaincode) {
	if !n.topology.FPC {
		return
	}

	orderer := n.Orderer("orderer")
	peers := n.PeersByName(chaincode.Peers)
	// Install on each peer its own chaincode package
	for _, peer := range peers {
		org := n.Organization(peer.Organization)
		chaincode.Chaincode.PackageFile = filepath.Join(n.ERCCPackagePath(), fmt.Sprintf("ercc.%s.%s.tgz", peer.Name, org.Domain))
		InstallChaincode(n, &chaincode.Chaincode, n.PeersByName([]string{peer.Name})...)
	}
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
}

func (n *Network) ERCCPackagePath() string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, "fpc", "ercc")
}
