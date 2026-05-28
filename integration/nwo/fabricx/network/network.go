/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"fmt"
	"path/filepath"
	"strconv"
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
)

var logger = logging.MustGetLogger()

const (
	DefaultConsensusType     = "etcdraft"
	defaultEventuallyTimeout = 60 * time.Second

	// namespacePropagationTimeout is the maximum time to wait for deployed
	// namespaces to become visible through the SC query service.
	namespacePropagationTimeout = 30 * time.Second

	// QueryServicePortName is the port name for the fabric-x committer query service
	QueryServicePortName = "QueryService"
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
	n.GenerateCryptoConfig()
	n.GenerateConfigTxConfig()
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

	// Extensions
	for _, extension := range n.Extensions {
		extension.PostRun(load)
	}

	logger.Infof("Next up: Deploying namespaces")
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
	c := createNSCommon(n, chaincode)
	cmd := &fxconfig.CreateNamespace{NamespaceCommon: c}
	sess, err := n.StartSession(common.NewCommand(fxconfig.CMDPath(), cmd), cmd.SessionName())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0), "fxconfig command failed: %s", string(sess.Err.Contents()))
}

// UpdateNamespace deploys the new version of the chaincode passed by chaincodeId.
func (n *Network) UpdateNamespace(chaincode *topology.ChannelChaincode) {
	v, err := strconv.Atoi(chaincode.Chaincode.Version)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	c := createNSCommon(n, chaincode)
	cmd := &fxconfig.UpdateNamespace{
		NamespaceCommon: c,
		Version:         v,
	}
	sess, err := n.StartSession(common.NewCommand(fxconfig.CMDPath(), cmd), cmd.SessionName())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0), "fxconfig command failed: %s", string(sess.Err.Contents()))
}

func createNSCommon(n *Network, chaincode *topology.ChannelChaincode) fxconfig.NamespaceCommon {
	orgName, err := namespaceApproverOrg(n)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	peers := n.PeersInOrg(orgName)
	gomega.Expect(peers).NotTo(gomega.BeEmpty())

	adminMspID := n.Organization(orgName).MSPID
	adminMspDir := n.PeerUserMSPDir(peers[0], "Admin")

	// get notification service endpoint
	committerNode := n.Peer(orgName, "SC")
	gomega.Expect(committerNode).NotTo(gomega.BeNil())

	notificationsEndpoint := fmt.Sprintf("127.0.0.1:%d", n.PeerPort(committerNode, fabric_network.ListenPort))

	var tlsConfig fxconfig.TLSConfig
	if n.TLSEnabled {
		tlsDir := n.PeerUserTLSDir(peers[0], "Admin")
		tlsConfig = fxconfig.TLSConfig{
			Enabled:        n.TLSEnabled,
			RootCerts:      []string{n.CACertsBundlePath()},
			ClientCertPath: filepath.Join(tlsDir, "client.crt"),
			ClientKeyPath:  filepath.Join(tlsDir, "client.key"),
		}
	}

	return fxconfig.NamespaceCommon{
		Name:    chaincode.Chaincode.Name,
		Channel: chaincode.Channel,
		MSPConfig: fxconfig.MSPConfig{
			ConfigPath: adminMspDir,
			LocalMspID: adminMspID,
		},
		OrdererConfig: fxconfig.OrdererConfig{
			Address: n.OrdererAddress(n.Orderers[0], fabric_network.ListenPort),
		},
		NotificationsConfig: fxconfig.NotificationsConfig{
			Address: notificationsEndpoint,
		},
		TLSConfig: tlsConfig,
		Policy:    chaincode.Chaincode.Policy,
	}
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
	committerNode := n.Peer(orgName, "SC")
	if committerNode == nil {
		return nil, fmt.Errorf("no committer peer (name=%v) found for org=%v", "SC", orgName)
	}
	queryEndpoint := fmt.Sprintf("127.0.0.1:%d", n.PeerPort(committerNode, QueryServicePortName))

	var tlsConfig fxconfig.TLSConfig
	if n.TLSEnabled {
		tlsDir := n.PeerUserTLSDir(peers[0], "Admin")
		tlsConfig = fxconfig.TLSConfig{
			Enabled:        n.TLSEnabled,
			RootCerts:      []string{n.CACertsBundlePath()},
			ClientCertPath: filepath.Join(tlsDir, "client.crt"),
			ClientKeyPath:  filepath.Join(tlsDir, "client.key"),
		}
	}

	cmd := &fxconfig.ListNamespaces{
		QueryConfig: fxconfig.QueryConfig{
			Address: queryEndpoint,
		},
		TLSConfig: tlsConfig,
	}
	sess, err := n.StartSession(common.NewCommand(fxconfig.CMDPath(), cmd), cmd.SessionName())
	if err != nil {
		return nil, err
	}
	gomega.Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit())
	if sess.ExitCode() != 0 {
		return nil, fmt.Errorf("namespace list returned non-zero exit code %d: %s", sess.ExitCode(), string(sess.Err.Contents()))
	}
	return parseNamespaceList(string(sess.Out.Contents())), nil
}

// ListInstalledNames queries the SC query service for deployed namespaces.
// It panics (via gomega assertions) on any error. For polling use inside
// gomega.Eventually, use tryListInstalledNames instead.
func (n *Network) ListInstalledNames() []Namespace {
	namespaces, err := n.tryListInstalledNames()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to list namespaces")
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

	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		// Skip header, empty lines, and error messages
		if line == "" ||
			strings.HasPrefix(line, "Installed namespaces") ||
			strings.HasPrefix(line, "Error:") ||
			strings.HasPrefix(line, "Usage:") ||
			strings.HasPrefix(line, "Flags:") {
			continue
		}

		// Look for name and version in the line
		// Expected format: "N) name: version X policy: <hex>"
		idx := strings.Index(line, ")")
		if idx > 0 {
			rest := strings.TrimSpace(line[idx+1:])
			colonIdx := strings.Index(rest, ":")
			if colonIdx > 0 {
				name := strings.TrimSpace(rest[:colonIdx])

				// Extract version
				vIdx := strings.Index(rest, " version ")
				if vIdx > 0 {
					vPart := strings.TrimSpace(rest[vIdx+len(" version "):])
					spaceIdx := strings.Index(vPart, " ")
					if spaceIdx != -1 {
						vPart = vPart[:spaceIdx]
					}
					version := 0
					n, err := fmt.Sscanf(vPart, "%d", &version)
					if err != nil || n != 1 {
						logger.Warnf("Failed to parse version from '%s': %v, skipping entry", vPart, err)
						continue
					}

					namespaces = append(namespaces, Namespace{
						Name:    name,
						Version: version,
					})
				}
			}
		}
	}

	if len(namespaces) == 0 && len(output) > 0 {
		logger.Debugf("Failed to parse any namespaces from output: %s", output)
	}

	return namespaces
}
