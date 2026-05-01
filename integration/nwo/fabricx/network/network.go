/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/fxconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

var logger = logging.MustGetLogger()

const (
	DefaultConsensusType     = "etcdraft"
	scVersionKey             = "sc_version"
	defaultEventuallyTimeout = 10 * time.Second

	// namespacePropagationTimeout is the maximum time to wait for deployed
	// namespaces to become visible through the SC query service.
	namespacePropagationTimeout = 30 * time.Second
)

type Network struct {
	*fabric_network.Network
}

func New(reg api.Context, topology *topology.Topology, builderClient fabric_network.BuilderClient, ccps []fabric_network.ChaincodeProcessor, networkID string) *Network {
	fabricNetwork := fabric_network.New(reg, topology, builderClient, ccps, networkID)
	fabricNetwork.EventuallyTimeout = defaultEventuallyTimeout
	n := &Network{Network: fabricNetwork}
	return n
}

func (n *Network) GenerateConfigTree() {
	n.CheckTopology()
	n.Network.CheckTopology()

	// TODO: remove this
	o := n.Orderers[0]
	ports := n.Context.PortsByOrdererID(n.Prefix, o.ID())
	ports[fabric_network.ListenPort] = 7050

	n.Context.SetPortsByOrdererID(n.Prefix, o.ID(), ports)
	n.Context.SetHostByOrdererID(n.Prefix, o.ID(), "127.0.0.1")

	// generate cyroto
	n.GenerateCryptoConfig()

	// generate genesis blocks
	// n.WriteConfigTxConfig()
	n.GenerateConfigTxConfig()

	// err := generateConfigTxYaml(n)
	// utils.Must(err)
}

func (n *Network) GenerateArtifacts() {
	// generate crypto material
	sess, err := n.Cryptogen(commands.Generate{
		NetworkPrefix: n.Prefix,
		Config:        n.CryptoConfigPath(),
		Output:        n.CryptoPath(),
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	bootstrapIdemix(n)
	bootstrapExtraIdentities(n)

	// we use the approver initially as out metanamespace EP
	// TODO: eventually we will set the metanamespace key based on the channel EP
	err = createMetanamespaceKey(n)
	utils.Must(err)

	// create channels
	for _, c := range n.Channels {
		sess, err = n.ConfigTxGen(createChannelBlock(n, c))
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}

	n.ConcatenateTLSCACertificates()
	n.GenerateResolverMap()
	for _, p := range n.Peers {
		if p.Type == topology.FSCPeer {
			n.GenerateCoreConfig(p)
		}
	}

	// Extensions
	for _, extension := range n.Extensions {
		extension.GenerateArtifacts()
	}
}

func (n *Network) Members() []grouper.Member {
	// note that we do not start any peers or orderers here
	// the committer all-in-one image takes care of this
	return grouper.Members{}
}

// CheckTopology checks the topology of the network.
func (n *Network) CheckTopology() {
	if n.Templates == nil {
		n.Templates = &topology.Templates{}
	}
	if n.Logging == nil {
		n.Logging = &topology.Logging{
			Spec:   "grpc=error:fabricx=debug:info",
			Format: "'%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'",
		}
	}

	// set ordering service to ARMA
	n.Consensus.Type = DefaultConsensusType

	// remove fabric peers
	n.removePeers()

	// cleanup chaincode
	// TODO cleanup the chaincode
	n.CheckTopologyOrderers()
	n.CheckTopologyExtensions()
}

func (n *Network) removePeers() {
	n.Peers = make([]*topology.Peer, 0)
}

func (n *Network) PostRun(load bool) {
	logger.Infof("Post execution [%s]...", n.Prefix)

	// NOTE: we skip the orderer join chanel as the committer test image deals with this
	// if !load {
	//	orderer := n.Orderer("orderer")
	//	for _, channel := range n.Channels {
	//		// orderer join the channel
	//		n.OrdererJoinChannel(channel.Name, orderer)
	//	}
	//
	//	// Wait a few second to make peers discovering each other
	//	time.Sleep(5 * time.Second)
	//}

	// Extensions
	for _, extension := range n.Extensions {
		extension.PostRun(load)
	}

	logger.Infof("Next up: Deploying namespaces")
	time.Sleep(n.EventuallyTimeout)

	expNss := make([]Namespace, 0, len(n.Topology().Chaincodes))
	for _, chaincode := range n.Topology().Chaincodes {
		n.DeployNamespace(chaincode)
		expNss = append(expNss, Namespace{Name: chaincode.Chaincode.Name, Version: 0})
	}

	// List all deployed namespaces and verify they are available.
	gomega.Eventually(n.tryListInstalledNames).WithTimeout(namespacePropagationTimeout).ProbeEvery(2 * time.Second).Should(gomega.ContainElements(expNss))
	logger.Infof("Post execution [%s]...done.", n.Prefix)
}

func (n *Network) DeployNamespace(chaincode *topology.ChannelChaincode) {
	orgName, err := namespaceApproverOrg(n)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	peers := n.PeersInOrg(orgName)
	gomega.Expect(peers).NotTo(gomega.BeEmpty())

	adminMspID := n.Organization(orgName).MSPID
	adminMspDir := n.PeerUserMSPDir(peers[0], "Admin")

	// get notification service endpoint
	committerNode := n.Peer(orgName, "SC")
	gomega.Expect(committerNode).NotTo(gomega.BeNil())

	committerSidecarPort := fmt.Sprintf("%d", n.PeerPort(committerNode, fabric_network.ListenPort))
	notificationsEndpoint := net.JoinHostPort("localhost", committerSidecarPort)

	cmd := &fxconfig.CreateNamespace{
		NamespaceCommon: fxconfig.NamespaceCommon{
			Name:    chaincode.Chaincode.Name,
			Channel: chaincode.Channel,
			MSPConfig: fxconfig.MSPConfig{
				ConfigPath: adminMspDir,
				LocalMspID: adminMspID,
			},
			OrdererConfig: fxconfig.OrdererConfig{
				Address: n.OrdererAddress(n.Orderers[0], fabric_network.ListenPort),
				TLSConfig: fxconfig.TLSConfig{
					Enabled:        n.TLSEnabled,
					RootCerts:      []string{n.CACertsBundlePath()},
					ClientCertPath: filepath.Join(n.PeerUserTLSDir(peers[0], "Admin"), "client.crt"),
					ClientKeyPath:  filepath.Join(n.PeerUserTLSDir(peers[0], "Admin"), "client.key"),
				},
			},
			NotificationsConfig: fxconfig.NotificationsConfig{
				Address: notificationsEndpoint,
				TLSConfig: fxconfig.TLSConfig{
					Enabled:        n.TLSEnabled,
					RootCerts:      []string{n.CACertsBundlePath()},
					ClientCertPath: filepath.Join(n.PeerUserTLSDir(peers[0], "Admin"), "client.crt"),
					ClientKeyPath:  filepath.Join(n.PeerUserTLSDir(peers[0], "Admin"), "client.key"),
				},
			},
			Policy: chaincode.Chaincode.Policy,
		},
	}
	sess, err := n.StartSession(common.NewCommand(fxconfig.CMDPath(), cmd), cmd.SessionName())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
}

// UpdateNamespace deploys the new version of the chaincode passed by chaincodeId.
func (n *Network) UpdateNamespace(chaincodeID, version, path, packageFile string) {
	// TODO:
}

// tryListInstalledNames is a polling-safe variant of ListInstalledNames.
// It returns an empty slice on any error (command start failure or non-zero
// exit code) instead of panicking, making it safe to use inside
// gomega.Eventually for retrying.
func (n *Network) tryListInstalledNames() ([]Namespace, error) {
	orgName, err := namespaceApproverOrg(n)
	if err != nil {
		return nil, err
	}
	peers := n.PeersInOrg(orgName)
	if len(peers) == 0 {
		return nil, fmt.Errorf("no peers found for org %s", orgName)
	}

	cmd := &fxconfig.ListNamespaces{QueryConfig: fxconfig.QueryConfig{
		Address: "127.0.0.1:7001",
		TLSConfig: fxconfig.TLSConfig{
			Enabled:        n.TLSEnabled,
			RootCerts:      []string{n.CACertsBundlePath()},
			ClientCertPath: filepath.Join(n.PeerUserTLSDir(peers[0], "Admin"), "client.crt"),
			ClientKeyPath:  filepath.Join(n.PeerUserTLSDir(peers[0], "Admin"), "client.key"),
		},
	}}
	sess, err := n.StartSession(common.NewCommand(fxconfig.CMDPath(), cmd), cmd.SessionName())
	if err != nil {
		return nil, err
	}
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
	if sess.ExitCode() != 0 {
		return nil, fmt.Errorf("namespace list returned non-zero exit code %d", sess.ExitCode())
	}
	return parseNamespaceList(string(sess.Out.Contents())), nil
}

// ListInstalledNames queries the SC query service for deployed namespaces.
// It panics (via gomega assertions) on any error. For polling use inside
// gomega.Eventually, use tryListInstalledNames instead.
func (n *Network) ListInstalledNames() []Namespace {
	namespaces, err := n.tryListInstalledNames()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return namespaces
}

type Namespace struct {
	Name    string
	Version int
}

// parseNamespaceList parses the output of 'fxconfig namespace list' command
// Expected format: "N) name: version X policy: <hex>"
// Example: "0) perf: version 0 policy: 0a05454344534112b201..."
func parseNamespaceList(output string) []Namespace {
	namespaces := make([]Namespace, 0)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// Skip header, empty lines, and error messages
		if line == "" ||
			strings.HasPrefix(line, "Installed namespaces") ||
			strings.HasPrefix(line, "Error:") ||
			strings.HasPrefix(line, "Usage:") ||
			strings.HasPrefix(line, "Flags:") {
			continue
		}

		// Parse line format: "0) perf: version 0 policy: ..."
		if idx := strings.Index(line, ")"); idx > 0 {
			rest := strings.TrimSpace(line[idx+1:])

			// Split by "version" keyword
			parts := strings.Split(rest, " version ")
			if len(parts) != 2 {
				continue
			}

			// Extract name (before ":")
			namePart := strings.TrimSpace(parts[0])
			if colonIdx := strings.Index(namePart, ":"); colonIdx > 0 {
				name := strings.TrimSpace(namePart[:colonIdx])

				// Extract version (ignore policy)
				versionPart := strings.TrimSpace(parts[1])
				versionPolicyParts := strings.Split(versionPart, " policy: ")

				version := 0
				_, err := fmt.Sscanf(versionPolicyParts[0], "%d", &version)
				if err != nil {
					logger.Warnf("Failed to parse version from '%s': %v, skipping entry", versionPolicyParts[0], err)
					continue
				}

				namespaces = append(namespaces, Namespace{
					Name:    name,
					Version: version,
				})
			}
		}
	}

	return namespaces
}
