/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

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
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger/fabric-private-chaincode/client_sdk/go/pkg/core/lifecycle"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/fabricconfig"
	nnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/packager"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

type ChaincodeInput struct {
	Args []string `json:"Args"`
}

type Connection struct {
	Address     string `json:"address"`
	DialTimeout string `json:"dial_timeout"`
	TlsRequired bool   `json:"tls_required"`
}

type Extension struct {
	network *nnetwork.Network

	ports   map[string][]uint16
	FPCERCC *topology.ChannelChaincode
}

func NewExtension(network *nnetwork.Network) *Extension {
	return &Extension{
		network: network,
		ports:   map[string][]uint16{},
	}
}

func (n *Extension) CheckTopology() {
	if !n.network.Topology().FPC {
		return
	}

	// Define External Builder
	cwd, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())
	index := strings.LastIndex(cwd, "github.com/hyperledger-labs/fabric-smart-client")
	Expect(index).ToNot(BeEquivalentTo(-1))

	path := cwd[:index] + "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/fpc/externalbuilders/chaincode_server"
	n.network.ExternalBuilders = append(n.network.ExternalBuilders, fabricconfig.ExternalBuilder{
		Path:                 path,
		Name:                 "fpc-external-launcher",
		PropagateEnvironment: []string{"CORE_PEER_ID", "FABRIC_LOGGING_SPEC"},
	})

	// Reserve ports for each private chaincode
	for _, chaincode := range n.network.Topology().Chaincodes {
		if !chaincode.Private {
			continue
		}

		if chaincode.Chaincode.Name == "ercc" {
			n.FPCERCC = chaincode
		}

		var ports []uint16
		for range chaincode.Peers {
			ports = append(ports, n.network.Context.ReservePort())
		}

		n.ports[chaincode.Chaincode.Name] = ports
	}

	return
}

func (n *Extension) GenerateArtifacts() {
	for _, chaincode := range n.network.Topology().Chaincodes {
		if !chaincode.Private {
			continue
		}
		// Generate Enclave Registry CC package for each peer that will conn
		Expect(os.MkdirAll(n.packagePath(chaincode.Chaincode.Name), 0770)).NotTo(HaveOccurred())

		peers := n.network.PeersByName(chaincode.Peers)
		for i, peer := range peers {
			org := n.network.Organization(peer.Organization)

			Expect(packager.New().PackageChaincode(
				chaincode.Chaincode.Name,
				chaincode.Chaincode.Lang,
				chaincode.Chaincode.Label,
				filepath.Join(n.packagePath(chaincode.Chaincode.Name), fmt.Sprintf("%s.%s.%s.tgz", chaincode.Chaincode.Name, peer.Name, org.Domain)),
				func(s string, s2 string) (string, []byte) {
					if strings.HasSuffix(s, "connection.json") {
						raw, err := json.MarshalIndent(&Connection{
							Address:     "127.0.0.1:" + strconv.Itoa(int(n.ports[chaincode.Chaincode.Name][i])),
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

	for _, org := range n.network.PeerOrgs() {
		var p *topology.Peer
		for _, peer := range n.network.Peers {
			if peer.Organization == org.Name {
				p = peer
				break
			}
		}
		Expect(p).NotTo(BeNil())

	}
}

func (n *Extension) PostRun() {
	if !n.network.Topology().FPC {
		return
	}

	if len(n.network.Topology().Chaincodes) != 0 {
		for _, chaincode := range n.network.Topology().Chaincodes {
			if chaincode.Private {
				n.deployChaincode(chaincode)
			}
		}
	}
}

// FPCCheckTopology ensures that the topology contains what is necessary to deployFPC
func (n *Extension) checkTopology() {
}

func (n *Extension) deployChaincode(chaincode *topology.ChannelChaincode) {
	if !n.network.Topology().FPC {
		return
	}

	orderer := n.network.Orderer("orderer")
	peers := n.network.PeersByName(chaincode.Peers)
	// Install on each peer its own chaincode package
	var packageIDs []string
	for _, peer := range peers {
		chaincode.Chaincode.PackageID = ""
		org := n.network.Organization(peer.Organization)
		chaincode.Chaincode.PackageFile = filepath.Join(
			n.packagePath(chaincode.Chaincode.Name),
			fmt.Sprintf("%s.%s.%s.tgz", chaincode.Chaincode.Name, peer.Name, org.Domain),
		)
		nnetwork.InstallChaincode(n.network, &chaincode.Chaincode, peer)
		nnetwork.ApproveChaincodeForMyOrg(n.network, chaincode.Channel, orderer, &chaincode.Chaincode, peer)
		packageIDs = append(packageIDs, chaincode.Chaincode.PackageID)
	}

	n.runDockerContainers(chaincode, packageIDs)

	nnetwork.CheckCommitReadinessUntilReady(n.network, chaincode.Channel, &chaincode.Chaincode, n.network.PeerOrgsByPeers(peers), peers...)
	nnetwork.CommitChaincode(n.network, chaincode.Channel, orderer, &chaincode.Chaincode, peers[0], peers...)
	time.Sleep(10 * time.Second)
	for i, peer := range peers {
		nnetwork.QueryInstalledReferences(n.network,
			chaincode.Channel, chaincode.Chaincode.Label, packageIDs[i],
			peer,
			[]string{chaincode.Chaincode.Name, chaincode.Chaincode.Version},
		)
	}
	if chaincode.Chaincode.InitRequired {
		nnetwork.InitChaincode(n.network, chaincode.Channel, orderer, &chaincode.Chaincode, peers...)
	}

	// initEnclave if not an ERCC
	if chaincode.Chaincode.Name != "ercc" {
		n.initEnclave(chaincode)
	}
}

func (n *Extension) packagePath(suffix string) string {
	return filepath.Join(n.network.Context.RootDir(), n.network.Prefix, "fpc", suffix)
}

func (n *Extension) runDockerContainers(chaincode *topology.ChannelChaincode, packageIDs []string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	peers := n.network.PeersByName(chaincode.Peers)
	for i, peer := range peers {
		port := strconv.Itoa(int(n.ports[chaincode.Chaincode.Name][i]))

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
			Mounts: []mount.Mount{
				{
					Type: mount.TypeBind,
					// Absolute path to
					Source: "/dev/null",
					Target: "/dev/null",
				},
			}, Resources: container.Resources{
				Devices: []container.DeviceMapping{
					{
						PathOnHost:        "/dev/null",
						PathInContainer:   "/dev/null",
						CgroupPermissions: "rwm",
					},
				},
			},
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
			fmt.Sprintf("%s.%s.%s.%s",
				// "hello_world_prefix",
				n.network.NetworkID,
				chaincode.Chaincode.Name, peer.Name,
				n.network.Organization(peer.Organization).Domain),
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})).ToNot(HaveOccurred())
		time.Sleep(3 * time.Second)
	}
}

func (n *Extension) initEnclave(chaincode *topology.ChannelChaincode) {
	initOrgs := map[string]bool{}
	var erccPeerAddresses []string
	peers := n.network.PeersByName(n.FPCERCC.Peers)
	for _, p := range peers {
		if exists := initOrgs[p.Organization]; !exists {
			erccPeerAddresses = append(erccPeerAddresses, n.network.PeerAddress(p, nnetwork.ListenPort))
			initOrgs[p.Organization] = true
		}
	}

	orderer := n.network.Orderer("orderer")
	peers = n.network.PeersByName(chaincode.Peers)
	attestationParams := CreateSIMAttestationParams()
	for _, peer := range peers {
		lifecycleClient, err := lifecycle.New(func(channelID string) (lifecycle.ChannelClient, error) {
			return &channelClient{
				n:         n,
				peer:      peer,
				orderer:   orderer,
				chaincode: chaincode,
			}, nil
		})
		Expect(err).ToNot(HaveOccurred())
		_, err = lifecycleClient.LifecycleInitEnclave(chaincode.Channel, lifecycle.LifecycleInitEnclaveRequest{
			ChaincodeID:         chaincode.Chaincode.Name,
			EnclavePeerEndpoint: erccPeerAddresses[0],
			AttestationParams:   attestationParams,
		})
		Expect(err).ToNot(HaveOccurred())

		// Enclave Init should happen once only
		return
	}
}

type channelClient struct {
	n         *Extension
	peer      *topology.Peer
	orderer   *topology.Orderer
	chaincode *topology.ChannelChaincode
}

func (c *channelClient) Query(chaincodeID string, fcn string, args [][]byte, targetEndpoints ...string) ([]byte, error) {
	ci := &ChaincodeInput{
		Args: append(append([]string{}, fcn), toStrings(args)...),
	}
	ctor, err := json.Marshal(ci)
	Expect(err).ToNot(HaveOccurred())

	sess, err := c.n.network.PeerUserSession(c.peer, "User1", commands.ChaincodeQuery{
		ChannelID: c.chaincode.Channel,
		Name:      chaincodeID,
		Ctor:      string(ctor),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, c.n.network.EventuallyTimeout).Should(gexec.Exit(0))

	return sess.Buffer().Contents(), nil
}

func (c *channelClient) Execute(chaincodeID string, fcn string, args [][]byte) (string, error) {
	ci := &ChaincodeInput{
		Args: append(append([]string{}, fcn), toStrings(args)...),
	}
	ctor, err := json.Marshal(ci)
	Expect(err).ToNot(HaveOccurred())

	initOrgs := map[string]bool{}
	var erccPeerAddresses []string
	peers := c.n.network.PeersByName(c.n.FPCERCC.Peers)
	for _, p := range peers {
		if exists := initOrgs[p.Organization]; !exists {
			erccPeerAddresses = append(erccPeerAddresses, c.n.network.PeerAddress(p, nnetwork.ListenPort))
			initOrgs[p.Organization] = true
		}
	}

	sess, err := c.n.network.PeerUserSession(c.peer, "User1", commands.ChaincodeInvoke{
		NetworkPrefix: c.n.network.Prefix,
		ChannelID:     c.chaincode.Channel,
		Orderer:       c.n.network.OrdererAddress(c.orderer, nnetwork.ListenPort),
		Name:          chaincodeID,
		Ctor:          string(ctor),
		PeerAddresses: erccPeerAddresses,
		WaitForEvent:  true,
		ClientAuth:    c.n.network.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, c.n.network.EventuallyTimeout).Should(gexec.Exit(0))
	for i := 0; i < len(erccPeerAddresses); i++ {
		Eventually(sess.Err, c.n.network.EventuallyTimeout).Should(gbytes.Say(`\Qcommitted with status (VALID)\E`))
	}
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	return "", nil
}

func toStrings(args [][]byte) []string {
	res := make([]string, len(args))
	for _, arg := range args {
		res = append(res, string(arg))
	}
	return res
}
