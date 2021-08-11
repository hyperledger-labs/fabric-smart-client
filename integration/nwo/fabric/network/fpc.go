/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
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

	// Reserve ports for each private chaincode
	for _, chaincode := range n.topology.Chaincodes {
		if !chaincode.Private {
			continue
		}

		var ports []uint16
		for range chaincode.Peers {
			ports = append(ports, n.Context.ReservePort())
		}

		n.FPCPorts[chaincode.Chaincode.Name] = ports
	}
}

// GenerateArtifactsFPC generates FPC related artifacts
func (n *Network) GenerateArtifactsFPC() {
	for _, chaincode := range n.topology.Chaincodes {
		if !chaincode.Private {
			continue
		}
		// Generate Enclave Registry CC package for each peer that will conn
		Expect(os.MkdirAll(n.PackagePath(chaincode.Chaincode.Name), 0770)).NotTo(HaveOccurred())

		peers := n.PeersByName(chaincode.Peers)
		for i, peer := range peers {
			org := n.Organization(peer.Organization)

			Expect(packager.New().PackageChaincode(
				chaincode.Chaincode.Name,
				chaincode.Chaincode.Lang,
				chaincode.Chaincode.Label,
				filepath.Join(n.PackagePath(chaincode.Chaincode.Name), fmt.Sprintf("%s.%s.%s.tgz", chaincode.Chaincode.Name, peer.Name, org.Domain)),
				func(s string, s2 string) (string, []byte) {
					if strings.HasSuffix(s, "connection.json") {
						raw, err := json.MarshalIndent(&Connection{
							Address:     "127.0.0.1:" + strconv.Itoa(int(n.FPCPorts[chaincode.Chaincode.Name][i])),
							DialTimeout: "10s",
							TlsRequired: false,
						}, "", " ")
						Expect(err).NotTo(HaveOccurred())
						return filepath.Join(fmt.Sprintf("%s.%s", peer.Name, org.Domain), "connection.json"), raw
					}
					return "", nil
				},
			)).ToNot(HaveOccurred())
		}
	}

	for _, org := range n.PeerOrgs() {
		var p *topology.Peer
		for _, peer := range n.Peers {
			if peer.Organization == org.Name {
				p = peer
				break
			}
		}
		Expect(p).NotTo(BeNil())

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
	var packageIDs []string
	for _, peer := range peers {
		chaincode.Chaincode.PackageID = ""
		org := n.Organization(peer.Organization)
		chaincode.Chaincode.PackageFile = filepath.Join(
			n.PackagePath(chaincode.Chaincode.Name),
			fmt.Sprintf("%s.%s.%s.tgz", chaincode.Chaincode.Name, peer.Name, org.Domain),
		)
		InstallChaincode(n, &chaincode.Chaincode, peer)
		ApproveChaincodeForMyOrg(n, chaincode.Channel, orderer, &chaincode.Chaincode, peer)
		packageIDs = append(packageIDs, chaincode.Chaincode.PackageID)
	}

	n.RunDockerContainers(chaincode, packageIDs)

	CheckCommitReadinessUntilReady(n, chaincode.Channel, &chaincode.Chaincode, n.PeerOrgsByPeers(peers), peers...)
	CommitChaincode(n, chaincode.Channel, orderer, &chaincode.Chaincode, peers[0], peers...)
	time.Sleep(10 * time.Second)
	for i, peer := range peers {
		QueryInstalledReferences(n,
			chaincode.Channel, chaincode.Chaincode.Label, packageIDs[i],
			peer,
			[]string{chaincode.Chaincode.Name, chaincode.Chaincode.Version},
		)
	}
	if chaincode.Chaincode.InitRequired {
		InitChaincode(n, chaincode.Channel, orderer, &chaincode.Chaincode, peers...)
	}
}

func (n *Network) PackagePath(suffix string) string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, "fpc", suffix)
}

func (n *Network) RunDockerContainers(chaincode *topology.ChannelChaincode, packageIDs []string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	peers := n.PeersByName(chaincode.Peers)
	for i, peer := range peers {
		port := strconv.Itoa(int(n.FPCPorts[chaincode.Chaincode.Name][i]))

		resp, err := cli.ContainerCreate(ctx, &container.Config{
			Image: chaincode.PrivateChaincode.Image,
			Tty:   false,
			Env: []string{
				"CHAINCODE_SERVER_ADDRESS=0.0.0.0:" + port,
				"CHAINCODE_PKG_ID=" + packageIDs[i],
				"FABRIC_LOGGING_SPEC=debug",
				"SGX_MODE=SIM",
			},
			ExposedPorts: nat.PortSet{
				nat.Port(port + "/tcp"): struct{}{},
			},
		}, &container.HostConfig{
			PortBindings: nat.PortMap{
				nat.Port(port + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "127.0.0.1",
						HostPort: port,
					},
				},
			},
		},
			nil, nil,
			fmt.Sprintf("%s.%s.%s", chaincode.Chaincode.Name, peer.Name, n.Organization(peer.Organization).Domain),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})).ToNot(HaveOccurred())
		time.Sleep(3 * time.Second)
	}
}
