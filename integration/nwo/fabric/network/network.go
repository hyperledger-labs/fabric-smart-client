/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"path/filepath"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/fabricconfig"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("fsc.integration.fabric")

type ChaincodeProcessor interface {
	Process(network *Network, cc *topology.ChannelChaincode) *topology.ChannelChaincode
}

type Network struct {
	Context            api.Context
	topology           *topology.Topology
	RootDir            string
	Prefix             string
	Builder            *Builder
	DockerClient       *docker.Client
	ExternalBuilders   []fabricconfig.ExternalBuilder
	NetworkID          string
	EventuallyTimeout  time.Duration
	MetricsProvider    string
	StatsdEndpoint     string
	ClientAuthRequired bool

	PortsByBrokerID   map[string]api.Ports
	PortsByOrdererID  map[string]api.Ports
	Logging           *topology.Logging
	ChaincodeMode     string
	PvtTxSupport      bool
	PvtTxCCSupport    bool
	MSPvtTxSupport    bool
	MSPvtCCSupport    bool
	FabTokenSupport   bool
	FabTokenCCSupport bool
	GRPCLogging       bool
	Organizations     []*topology.Organization
	SystemChannel     *topology.SystemChannel
	Channels          []*topology.Channel
	Consensus         *topology.Consensus
	Orderers          []*topology.Orderer
	Peers             []*topology.Peer
	Profiles          []*topology.Profile
	Consortiums       []*topology.Consortium
	Templates         *topology.Templates
	Resolvers         []*Resolver

	colorIndex uint
	ccps       []ChaincodeProcessor
}

func New(reg api.Context, topology *topology.Topology, dockerClient *docker.Client, builderClient BuilderClient, ccps []ChaincodeProcessor, NetworkID string) *Network {
	if topology == nil {
		topology = NewEmptyTopology()
	}

	network := &Network{
		Context:      reg,
		Builder:      &Builder{builderClient},
		DockerClient: dockerClient,
		RootDir:      reg.RootDir(),
		Prefix:       "fabric." + topology.Name(),
		topology:     topology,

		NetworkID:         NetworkID,
		EventuallyTimeout: 10 * time.Minute,
		MetricsProvider:   "prometheus",
		PortsByBrokerID:   map[string]api.Ports{},
		PortsByOrdererID:  map[string]api.Ports{},

		Organizations:     topology.Organizations,
		Consensus:         topology.Consensus,
		Orderers:          topology.Orderers,
		Peers:             topology.Peers,
		SystemChannel:     topology.SystemChannel,
		Channels:          topology.Channels,
		Profiles:          topology.Profiles,
		Consortiums:       topology.Consortiums,
		Templates:         topology.Templates,
		Logging:           topology.Logging,
		ChaincodeMode:     topology.ChaincodeMode,
		MSPvtTxSupport:    topology.MSPvtTxSupport,
		MSPvtCCSupport:    topology.MSPvtCCSupport,
		FabTokenSupport:   topology.FabTokenSupport,
		FabTokenCCSupport: topology.FabTokenCCSupport,
		GRPCLogging:       topology.GRPCLogging,
		PvtTxSupport:      topology.PvtTxSupport,
		PvtTxCCSupport:    topology.PvtTxCCSupport,
		ccps:              ccps,
	}
	return network
}

func (n *Network) GenerateConfigTree() {
	n.CheckTopology()
	n.GenerateCryptoConfig()
	if len(n.Channels) != 0 {
		n.GenerateConfigTxConfig()
	}
	for _, o := range n.Orderers {
		n.GenerateOrdererConfig(o)
	}
	for _, p := range n.Peers {
		if p.Type == topology.FabricPeer {
			n.GenerateCoreConfig(p)
		}
	}
}

func (n *Network) GenerateArtifacts() {
	sess, err := n.Cryptogen(commands.Generate{
		NetworkPrefix: n.Prefix,
		Config:        n.CryptoConfigPath(),
		Output:        n.CryptoPath(),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	n.bootstrapIdemix()
	n.bootstrapExtraIdentities()

	if len(n.SystemChannel.Name) != 0 {
		sess, err = n.ConfigTxGen(commands.OutputBlock{
			NetworkPrefix: n.Prefix,
			ChannelID:     n.SystemChannel.Name,
			Profile:       n.SystemChannel.Profile,
			ConfigPath:    filepath.Join(n.Context.RootDir(), n.Prefix),
			OutputBlock:   n.OutputBlockPath(n.SystemChannel.Name),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	for _, c := range n.Channels {
		sess, err := n.ConfigTxGen(commands.CreateChannelTx{
			NetworkPrefix:         n.Prefix,
			ChannelID:             c.Name,
			Profile:               c.Profile,
			BaseProfile:           c.BaseProfile,
			ConfigPath:            filepath.Join(n.Context.RootDir(), n.Prefix),
			OutputCreateChannelTx: n.CreateChannelTxPath(c.Name),
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	n.ConcatenateTLSCACertificates()
	n.GenerateResolverMap()
	for _, p := range n.Peers {
		switch p.Type {
		case topology.FSCPeer:
			n.GenerateCoreConfig(p)
		}
	}
}

func (n *Network) Load() {
	// Nothing to do here
}

func (n *Network) Members() []grouper.Member {
	members := grouper.Members{}

	if r := n.BrokerGroupRunner(); r != nil {
		members = append(members, grouper.Member{Name: n.Prefix + ".brokers", Runner: r})
	}
	if r := n.OrdererGroupRunner(); r != nil {
		members = append(members, grouper.Member{Name: n.Prefix + ".orderers", Runner: r})
	}
	if r := n.PeerGroupRunner(); r != nil {
		members = append(members, grouper.Member{Name: n.Prefix + ".peers", Runner: r})
	}
	return members
}

func (n *Network) PostRun() {
	logger.Infof("Post execution [%s]...", n.Prefix)
	orderer := n.Orderer("orderer")
	for _, channel := range n.Channels {
		n.CreateAndJoinChannel(orderer, channel.Name)
		n.UpdateChannelAnchors(orderer, channel.Name)
	}

	// Wait a few second to make peers discovering each other
	time.Sleep(5 * time.Second)

	// Install chaincodes, if needed
	if len(n.topology.Chaincodes) != 0 {
		for _, chaincode := range n.topology.Chaincodes {
			for _, ccp := range n.ccps {
				chaincode = ccp.Process(n, chaincode)
			}
			n.DeployChaincode(chaincode)
		}
	}

	// Wait a few second to make peers discovering each other
	time.Sleep(5 * time.Second)
	logger.Infof("Post execution [%s]...done.", n.Prefix)
}

func (n *Network) Cleanup() {
	if n.DockerClient == nil {
		return
	}

	nw, err := n.DockerClient.NetworkInfo(n.NetworkID)
	if _, ok := err.(*docker.NoSuchNetwork); err != nil && ok {
		return
	}
	Expect(err).NotTo(HaveOccurred())

	err = n.DockerClient.RemoveNetwork(nw.ID)
	Expect(err).NotTo(HaveOccurred())

	containers, err := n.DockerClient.ListContainers(docker.ListContainersOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.HasPrefix(name, "/"+n.NetworkID) {
				err := n.DockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID, Force: true})
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}

	images, err := n.DockerClient.ListImages(docker.ListImagesOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, i := range images {
		for _, tag := range i.RepoTags {
			if strings.HasPrefix(tag, n.NetworkID) {
				err := n.DockerClient.RemoveImage(i.ID)
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}
}

func (n *Network) DeployChaincode(chaincode *topology.ChannelChaincode) {
	orderer := n.Orderer("orderer")
	peers := n.PeersByName(chaincode.Peers)

	if len(chaincode.Chaincode.PackageFile) == 0 {
		if len(chaincode.Path) != 0 {
			chaincodePath := n.Builder.Build(chaincode.Path)
			chaincode.Chaincode.Path = chaincodePath
			chaincode.Chaincode.Lang = "binary"
		}
		chaincode.Chaincode.PackageFile = filepath.Join(n.Context.RootDir(), n.Prefix, chaincode.Chaincode.Name+".tar.gz")
		PackageChaincode(n, &chaincode.Chaincode, peers[0])
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
}
