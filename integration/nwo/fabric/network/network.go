/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"path/filepath"
	"strconv"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/fabricconfig"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/packager"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/packager/replacer"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit/grouper"
)

var logger = logging.MustGetLogger()

type ChaincodeProcessor interface {
	Process(network *Network, cc *topology.ChannelChaincode) *topology.ChannelChaincode
}

type Extension interface {
	CheckTopology()
	GenerateArtifacts()
	PostRun(load bool)
}

type Packager interface {
	PackageChaincode(path, typ, label, outputFile string, replacer replacer.Func) error
}

type PackagerFactory = func() Packager

type Network struct {
	Context                  api.Context
	topology                 *topology.Topology
	RootDir                  string
	Prefix                   string
	Builder                  *Builder
	ExternalBuilders         []fabricconfig.ExternalBuilder
	NetworkID                string
	EventuallyTimeout        time.Duration
	MetricsProvider          string
	StatsdEndpoint           string
	TLSEnabled               bool
	ClientAuthRequired       bool
	GatewayEnabled           bool
	OrdererReplicationPolicy string
	PeerDeliveryClientPolicy string
	UseWriteBatch            bool
	UseGetMultipleKeys       bool

	Logging           *topology.Logging
	PvtTxSupport      bool
	PvtTxCCSupport    bool
	MSPvtTxSupport    bool
	MSPvtCCSupport    bool
	FabTokenSupport   bool
	FabTokenCCSupport bool
	GRPCLogging       bool
	Organizations     []*topology.Organization
	Channels          []*topology.Channel
	Consensus         *topology.Consensus
	Orderers          []*topology.Orderer
	Peers             []*topology.Peer
	Profiles          []*topology.Profile
	Consortiums       []*topology.Consortium
	Templates         *topology.Templates
	Resolvers         []*Resolver

	Extensions      []Extension
	PackagerFactory PackagerFactory

	colorIndex uint
	ccps       []ChaincodeProcessor
}

func New(reg api.Context, topology *topology.Topology, builderClient BuilderClient, ccps []ChaincodeProcessor, NetworkID string) *Network {
	if topology == nil {
		topology = NewEmptyTopology()
	}

	network := &Network{
		Context:  reg,
		Builder:  &Builder{builderClient},
		RootDir:  reg.RootDir(),
		Prefix:   "fabric." + topology.Name(),
		topology: topology,

		NetworkID:         NetworkID,
		EventuallyTimeout: 20 * time.Minute,
		MetricsProvider:   "prometheus",

		Organizations:      topology.Organizations,
		Consensus:          topology.Consensus,
		Orderers:           topology.Orderers,
		Peers:              topology.Peers,
		Channels:           topology.Channels,
		Profiles:           topology.Profiles,
		Consortiums:        topology.Consortiums,
		Templates:          topology.Templates,
		Logging:            topology.Logging,
		MSPvtTxSupport:     topology.MSPvtTxSupport,
		MSPvtCCSupport:     topology.MSPvtCCSupport,
		FabTokenSupport:    topology.FabTokenSupport,
		FabTokenCCSupport:  topology.FabTokenCCSupport,
		GRPCLogging:        topology.GRPCLogging,
		PvtTxSupport:       topology.PvtTxSupport,
		PvtTxCCSupport:     topology.PvtTxCCSupport,
		ClientAuthRequired: topology.ClientAuthRequired,
		TLSEnabled:         topology.TLSEnabled,
		ccps:               ccps,
		Extensions:         []Extension{},
		PackagerFactory: func() Packager {
			return packager.New()
		},
	}
	return network
}

func (n *Network) GenerateConfigTree() {
	n.CheckTopology()
	n.GenerateCryptoConfig()
	if len(n.Channels) != 0 {
		// We should always have a channel no?
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
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	n.bootstrapIdemix()
	n.bootstrapExtraIdentities()

	for _, c := range n.Channels {
		sess, err = n.ConfigTxGen(n.createChannelBlock(c))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	n.ConcatenateTLSCACertificates()
	n.GenerateResolverMap()
	for _, p := range n.Peers {
		switch p.Type {
		case topology.FSCPeer:
			n.GenerateCoreConfig(p)
		}
	}

	// Extensions
	for _, extension := range n.Extensions {
		extension.GenerateArtifacts()
	}
}

func (n *Network) createChannelBlock(c *topology.Channel) common.Command {
	return commands.OutputBlock{
		NetworkPrefix: n.Prefix,
		ChannelID:     c.Name,
		Profile:       c.Profile,
		ConfigPath:    filepath.Join(n.Context.RootDir(), n.Prefix),
		OutputBlock:   n.OutputBlockPath(c.Name),
	}
}

func (n *Network) Load() {
}

func (n *Network) Members() []grouper.Member {
	members := grouper.Members{}

	if r := n.OrdererGroupRunner(); r != nil {
		members = append(members, grouper.Member{Name: n.Prefix + ".orderers", Runner: r})
	}
	if r := n.PeerGroupRunner(); r != nil {
		members = append(members, grouper.Member{Name: n.Prefix + ".peers", Runner: r})
	}
	return members
}

func (n *Network) PostRun(load bool) {
	logger.Infof("Post execution [%s]...", n.Prefix)

	if !load {
		orderer := n.Orderer("orderer")
		for _, channel := range n.Channels {
			// orderer join the channel
			n.OrdererJoinChannel(channel.Name, orderer)

			// peers join the channel
			n.JoinChannel(channel.Name, orderer, n.PeersWithChannel(channel.Name)...)
		}

		// Wait a few second to make peers discovering each other
		time.Sleep(5 * time.Second)

		// Install chaincodes, if needed
		if len(n.topology.Chaincodes) != 0 {
			for _, chaincode := range n.topology.Chaincodes {
				for _, ccp := range n.ccps {
					chaincode = ccp.Process(n, chaincode)
				}
				if !chaincode.Private {
					n.DeployChaincode(chaincode)
				}
			}
		}
	}

	// Extensions
	for _, extension := range n.Extensions {
		extension.PostRun(load)
	}

	// Wait a few second to let Fabric stabilize
	time.Sleep(5 * time.Second)
	logger.Infof("Post execution [%s]...done.", n.Prefix)
}

func (n *Network) Cleanup() {
	// DO nothing
}

func (n *Network) DeployChaincode(chaincode *topology.ChannelChaincode) {
	orderer := n.Orderer("orderer")
	peers := n.PeersForChaincodeByName(chaincode.Peers)

	if len(chaincode.Chaincode.PackageFile) == 0 {
		if len(chaincode.Path) != 0 {
			chaincodePath := n.Builder.Build(chaincode.Path)
			chaincode.Chaincode.Path = chaincodePath
			chaincode.Chaincode.Lang = "binary"
		}
		chaincode.Chaincode.PackageFile = filepath.Join(n.Context.RootDir(), n.Prefix, chaincode.Chaincode.Name+chaincode.Chaincode.Version+".tar.gz")
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
	// add new chaincode to the topology
	n.topology.AddChaincode(chaincode)
}

func (n *Network) AddExtension(ex Extension) {
	n.Extensions = append(n.Extensions, ex)
}

// UpdateChaincode deploys the new version of the chaincode passed by chaincodeId
func (n *Network) UpdateChaincode(chaincodeId string, version string, path string, packageFile string) {
	var cc *topology.ChannelChaincode
	for _, chaincode := range n.topology.Chaincodes {
		if chaincode.Chaincode.Name == chaincodeId {
			cc = chaincode
			break
		}
	}
	gomega.Expect(cc).ToNot(gomega.BeNil(), "failed to find chaincode [%s]", chaincodeId)

	seq, err := strconv.Atoi(cc.Chaincode.Sequence)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to parse chaincode sequence [%s]", cc.Chaincode.Sequence)

	newCC := &topology.ChannelChaincode{
		Chaincode: topology.Chaincode{
			Name:            cc.Chaincode.Name,
			Version:         version,
			Sequence:        strconv.Itoa(seq + 1),
			InitRequired:    cc.Chaincode.InitRequired,
			Path:            path,
			Lang:            cc.Chaincode.Lang,
			Label:           cc.Chaincode.Name,
			Ctor:            cc.Chaincode.Ctor,
			Policy:          cc.Chaincode.Policy,
			SignaturePolicy: cc.Chaincode.SignaturePolicy,
		},
		Channel: cc.Channel,
		Peers:   cc.Peers,
	}
	if len(packageFile) != 0 {
		newCC.Chaincode.PackageFile = packageFile
	}
	n.DeployChaincode(newCC)
}

func (n *Network) OrdererJoinChannel(channelID string, orderer *topology.Orderer) {
	cmd := commands.OSNAdminChannelJoin{
		NetworkPrefix:  n.Prefix,
		OrdererAddress: n.OrdererAddress(orderer, AdminPort),
		ChannelID:      channelID,
		BlockPath:      n.OutputBlockPath(channelID),
	}

	if n.TLSEnabled {
		tlsdir := n.OrdererLocalTLSDir(orderer)
		cmd.CAFile = filepath.Join(tlsdir, "ca.crt")
		cmd.ClientCert = filepath.Join(tlsdir, "server.crt")
		cmd.ClientKey = filepath.Join(tlsdir, "server.key")
	}

	sess, err := n.Osnadmin(cmd)

	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	time.Sleep(4 * time.Second)
	// TODO: get the orderer process so we can check when the msg has been processed
	//gomega.Eventually(ordererRunner.Err(), n.EventuallyTimeout, time.Second).Should(
	//	gbytes.Say(fmt.Sprintf("Raft leader changed: 0 -> 1 channel=%s node=1", channelID)))
}
