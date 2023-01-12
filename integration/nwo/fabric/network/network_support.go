/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/pkcs11"
	runner2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/fabricconfig"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"gopkg.in/yaml.v2"
)

func (n *Network) LogSpec() string {
	if len(n.Logging.Spec) == 0 {
		return "info:fscnode=debug"
	}
	return n.Logging.Spec
}

func (n *Network) LogFormat() string {
	if len(n.Logging.Format) == 0 {
		return "'%{color}%{time:2006-01-02 15:04:05.000 MST} [%{module}] %{shortfunc} -> %{level:.4s} %{id:03x}%{color:reset} %{message}'"
	}
	return n.Logging.Format
}

// AddOrg adds an organization to a network.
func (n *Network) AddOrg(o *topology.Organization, peers ...*topology.Peer) {
	for _, p := range peers {
		ports := api.Ports{}
		for _, portName := range PeerPortNames() {
			ports[portName] = n.Context.ReservePort()
		}
		n.Context.SetPortsByPeerID(n.Prefix, p.ID(), ports)
		n.Context.SetHostByPeerID(n.Prefix, p.ID(), "0.0.0.0")
		n.Peers = append(n.Peers, p)
	}

	n.Organizations = append(n.Organizations, o)
	n.Consortiums[0].Organizations = append(n.Consortiums[0].Organizations, o.Name)
}

// ConfigTxPath returns the path to the generated configtxgen configuration
// file.
func (n *Network) ConfigTxConfigPath() string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, "configtx.yaml")
}

// CryptoPath returns the path to the directory where cryptogen will place its
// generated artifacts.
func (n *Network) CryptoPath() string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, "crypto")
}

// CryptoConfigPath returns the path to the generated cryptogen configuration
// file.
func (n *Network) CryptoConfigPath() string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, "crypto-config.yaml")
}

// OutputBlockPath returns the path to the genesis block for the named system
// channel.
func (n *Network) OutputBlockPath(channelName string) string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, fmt.Sprintf("%s_block.pb", channelName))
}

// CreateChannelTxPath returns the path to the create channel transaction for
// the named channel.
func (n *Network) CreateChannelTxPath(channelName string) string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, fmt.Sprintf("%s_tx.pb", channelName))
}

// OrdererDir returns the path to the configuration directory for the specified
// Orderer.
func (n *Network) OrdererDir(o *topology.Orderer) string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, "orderers", o.ID())
}

// OrdererConfigPath returns the path to the orderer configuration document for
// the specified Orderer.
func (n *Network) OrdererConfigPath(o *topology.Orderer) string {
	return filepath.Join(n.OrdererDir(o), "orderer.yaml")
}

// ReadOrdererConfig  unmarshals an orderer's orderer.yaml and returns an
// object approximating its contents.
func (n *Network) ReadOrdererConfig(o *topology.Orderer) *fabricconfig.Orderer {
	var orderer fabricconfig.Orderer
	ordererBytes, err := ioutil.ReadFile(n.OrdererConfigPath(o))
	Expect(err).NotTo(HaveOccurred())

	err = yaml.Unmarshal(ordererBytes, &orderer)
	Expect(err).NotTo(HaveOccurred())

	return &orderer
}

// WriteOrdererConfig serializes the provided configuration as the specified
// orderer's orderer.yaml document.
func (n *Network) WriteOrdererConfig(o *topology.Orderer, config *fabricconfig.Orderer) {
	ordererBytes, err := yaml.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(n.OrdererConfigPath(o), ordererBytes, 0644)
	Expect(err).NotTo(HaveOccurred())
}

// ReadConfigTxConfig  unmarshals the configtx.yaml and returns an
// object approximating its contents.
func (n *Network) ReadConfigTxConfig() *fabricconfig.ConfigTx {
	var configtx fabricconfig.ConfigTx
	configtxBytes, err := ioutil.ReadFile(n.ConfigTxConfigPath())
	Expect(err).NotTo(HaveOccurred())

	err = yaml.Unmarshal(configtxBytes, &configtx)
	Expect(err).NotTo(HaveOccurred())

	return &configtx
}

// WriteConfigTxConfig serializes the provided configuration to configtx.yaml.
func (n *Network) WriteConfigTxConfig(config *fabricconfig.ConfigTx) {
	configtxBytes, err := yaml.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(n.ConfigTxConfigPath(), configtxBytes, 0644)
	Expect(err).NotTo(HaveOccurred())
}

func (n *Network) ConfigDir() string {
	return filepath.Join(n.Context.RootDir(), n.Prefix)
}

// PeerDir returns the path to the configuration directory for the specified
// Peer.
func (n *Network) PeerDir(p *topology.Peer) string {
	return filepath.Join(n.Context.RootDir(), n.Prefix, "peers", p.ID())
}

// PeerConfigPath returns the path to the peer configuration document for the
// specified peer.
func (n *Network) PeerConfigPath(p *topology.Peer) string {
	return filepath.Join(n.PeerDir(p), "core.yaml")
}

// PeerLedgerDir returns the rwset root directory for the specified peer.
func (n *Network) PeerLedgerDir(p *topology.Peer) string {
	return filepath.Join(n.PeerDir(p), "filesystem/ledgersData")
}

// ReadPeerConfig unmarshals a peer's core.yaml and returns an object
// approximating its contents.
func (n *Network) ReadPeerConfig(p *topology.Peer) *fabricconfig.Core {
	var core fabricconfig.Core
	coreBytes, err := ioutil.ReadFile(n.PeerConfigPath(p))
	Expect(err).NotTo(HaveOccurred())

	err = yaml.Unmarshal(coreBytes, &core)
	Expect(err).NotTo(HaveOccurred())

	return &core
}

// WritePeerConfig serializes the provided configuration as the specified
// peer's core.yaml document.
func (n *Network) WritePeerConfig(p *topology.Peer, config *fabricconfig.Core) {
	coreBytes, err := yaml.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(n.PeerConfigPath(p), coreBytes, 0644)
	Expect(err).NotTo(HaveOccurred())
}

// peerUserCryptoDir returns the path to the directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) peerUserCryptoDir(p *topology.Peer, user, cryptoMaterialType string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return n.userCryptoDir(org, "peerOrganizations", user, cryptoMaterialType)
}

// ordererUserCryptoDir returns the path to the directory containing the
// certificates and keys for the specified user of the orderer.
func (n *Network) ordererUserCryptoDir(o *topology.Orderer, user, cryptoMaterialType string) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return n.userCryptoDir(org, "ordererOrganizations", user, cryptoMaterialType)
}

// userCryptoDir returns the path to the folder with crypto materials for either peers or orderer organizations
// specific user
func (n *Network) userCryptoDir(org *topology.Organization, nodeOrganizationType, user, cryptoMaterialType string) string {
	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		nodeOrganizationType,
		org.Domain,
		"users",
		fmt.Sprintf("%s@%s", user, org.Domain),
		cryptoMaterialType,
	)
}

func (n *Network) OrgPeerCACertificatePath(org *topology.Organization) string {
	return n.orgCACertificatePath(org, "peerOrganizations")
}

func (n *Network) OrgOrdererCACertificatePath(org *topology.Organization) string {
	return n.orgCACertificatePath(org, "ordererOrganizations")
}

func (n *Network) OrgOrdererTLSCACertificatePath(org *topology.Organization) string {
	return n.orgTLSCACertificatePath(org, "ordererOrganizations")
}

func (n *Network) orgTLSCACertificatePath(org *topology.Organization, nodeOrganizationType string) string {
	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		nodeOrganizationType,
		org.Domain,
		"tlsca",
		fmt.Sprintf("tlsca.%s-cert.pem", org.Domain),
	)
}

func (n *Network) orgCACertificatePath(org *topology.Organization, nodeOrganizationType string) string {
	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		nodeOrganizationType,
		org.Domain,
		"ca",
		fmt.Sprintf("ca.%s-cert.pem", org.Domain),
	)
}

// PeerUserMSPDir returns the path to the MSP directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) PeerUserMSPDir(p *topology.Peer, user string) string {
	return n.peerUserCryptoDir(p, user, "msp")
}

func (n *Network) ViewNodeMSPDir(p *topology.Peer) string {
	switch {
	case n.topology.NodeOUs:
		switch p.Role {
		case "":
			return n.PeerUserMSPDir(p, p.Name)
		case "client":
			return n.PeerUserMSPDir(p, p.Name)
		default:
			return n.PeerLocalMSPDir(p)
		}
	default:
		return n.PeerLocalMSPDir(p)
	}
}

// FSCNodeLocalTLSDir returns the path to the local TLS directory for the peer.
func (n *Network) FSCNodeLocalTLSDir(p *topology.Peer) string {
	switch {
	case n.topology.NodeOUs:
		switch p.Role {
		case "":
			return n.peerUserCryptoDir(p, p.Name, "tls")
		case "client":
			return n.peerUserCryptoDir(p, p.Name, "tls")
		default:
			return n.peerLocalCryptoDir(p, "tls")
		}
	default:
		return n.peerLocalCryptoDir(p, "tls")
	}
}

// IdemixUserMSPDir returns the path to the MSP directory containing the
// idemix-related crypto material for the specified user of the organization.
func (n *Network) IdemixUserMSPDir(o *topology.Organization, user string) string {
	return n.userCryptoDir(o, "peerOrganizations", user, "")
}

// OrdererUserMSPDir returns the path to the MSP directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) OrdererUserMSPDir(o *topology.Orderer, user string) string {
	return n.ordererUserCryptoDir(o, user, "msp")
}

// PeerUserTLSDir returns the path to the TLS directory containing the
// certificates and keys for the specified user of the peer.
func (n *Network) PeerUserTLSDir(p *topology.Peer, user string) string {
	return n.peerUserCryptoDir(p, user, "tls")
}

// PeerUserCert returns the path to the certificate for the specified user in
// the peer organization.
func (n *Network) PeerUserCert(p *topology.Peer, user string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.PeerUserMSPDir(p, user),
		"signcerts",
		fmt.Sprintf("%s@%s-cert.pem", user, org.Domain),
	)
}

// OrdererUserCert returns the path to the certificate for the specified user in
// the orderer organization.
func (n *Network) OrdererUserCert(o *topology.Orderer, user string) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.OrdererUserMSPDir(o, user),
		"signcerts",
		fmt.Sprintf("%s@%s-cert.pem", user, org.Domain),
	)
}

// PeerUserKey returns the path to the private key for the specified user in
// the peer organization.
func (n *Network) PeerUserKey(p *topology.Peer, user string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.PeerUserMSPDir(p, user),
		"keystore",
		"priv_sk",
	)
}

func (n *Network) PeerKey(p *topology.Peer) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.PeerLocalMSPDir(p),
		"keystore",
		"priv_sk",
	)
}

// OrdererUserKey returns the path to the private key for the specified user in
// the orderer organization.
func (n *Network) OrdererUserKey(o *topology.Orderer, user string) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.OrdererUserMSPDir(o, user),
		"keystore",
		"priv_sk",
	)
}

// peerLocalCryptoDir returns the path to the local crypto directory for the peer.
func (n *Network) peerLocalCryptoDir(p *topology.Peer, cryptoType string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"peers",
		fmt.Sprintf("%s.%s", p.Name, org.Domain),
		cryptoType,
	)
}

func (n *Network) peerUserLocalCryptoDir(p *topology.Peer, user, cryptoType string) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"users",
		fmt.Sprintf("%s@%s", user, org.Domain),
		cryptoType,
	)
}

// PeerLocalMSPDir returns the path to the local MSP directory for the peer.
func (n *Network) PeerLocalMSPDir(p *topology.Peer) string {
	return n.peerLocalCryptoDir(p, "msp")
}

func (n *Network) PeerLocalMSPIdentityCert(p *topology.Peer) string {
	return filepath.Join(
		n.peerLocalCryptoDir(p, "msp"),
		"signcerts",
		p.Name+"."+n.Organization(p.Organization).Domain+"-cert.pem",
	)
}

func (n *Network) PeerLocalMSP(p *topology.Peer) string {
	return n.peerLocalCryptoDir(p, "msp")
}

func (n *Network) PeerUserLocalMSPIdentityCert(p *topology.Peer, user string) string {
	return filepath.Join(
		n.peerUserLocalCryptoDir(p, user, "msp"),
		"signcerts",
		p.Name+"@"+n.Organization(p.Organization).Domain+"-cert.pem",
	)
}

func (n *Network) PeerUserLocalMSP(p *topology.Peer, user string) string {
	return n.peerUserLocalCryptoDir(p, user, "msp")
}

func (n *Network) PeerLocalIdemixExtraIdentitiesDir(p *topology.Peer) string {
	return n.peerLocalCryptoDir(p, "extraids")
}

func (n *Network) PeerLocalExtraIdentityDir(p *topology.Peer, id string) string {
	for _, identity := range p.Identities {
		if identity.ID == id {
			switch identity.MSPType {
			case "idemix":
				return n.peerLocalCryptoDir(p, filepath.Join("extraids", id))
			case "bccsp":
				return n.peerUserCryptoDir(p, identity.ID, "msp")
			}
		}
	}
	panic("id not found")
}

// PeerLocalTLSDir returns the path to the local TLS directory for the peer.
func (n *Network) PeerLocalTLSDir(p *topology.Peer) string {
	return n.peerLocalCryptoDir(p, "tls")
}

// PeerCert returns the path to the peer's certificate.
func (n *Network) PeerCert(p *topology.Peer) string {
	org := n.Organization(p.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.PeerLocalMSPDir(p),
		"signcerts",
		fmt.Sprintf("%s.%s-cert.pem", p.Name, org.Domain),
	)
}

// PeerOrgMSPDir returns the path to the MSP directory of the Peer organization.
func (n *Network) PeerOrgMSPDir(org *topology.Organization) string {
	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		"peerOrganizations",
		org.Domain,
		"msp",
	)
}

func (n *Network) Topology() *topology.Topology {
	return n.topology
}

func (n *Network) DefaultIdemixOrgMSPDir() string {
	for _, organization := range n.Organizations {
		if organization.MSPType == "idemix" {
			return filepath.Join(
				n.Context.RootDir(),
				n.Prefix,
				"crypto",
				"peerOrganizations",
				organization.Domain,
			)
		}
	}
	return ""
}

func (n *Network) IdemixOrgMSPDir(org *topology.Organization) string {
	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		"peerOrganizations",
		org.Domain,
	)
}

// OrdererOrgMSPDir returns the path to the MSP directory of the Orderer
// organization.
func (n *Network) OrdererOrgMSPDir(o *topology.Organization) string {
	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		"ordererOrganizations",
		o.Domain,
		"msp",
	)
}

// OrdererLocalCryptoDir returns the path to the local crypto directory for the
// Orderer.
func (n *Network) OrdererLocalCryptoDir(o *topology.Orderer, cryptoType string) string {
	org := n.Organization(o.Organization)
	Expect(org).NotTo(BeNil())

	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		"ordererOrganizations",
		org.Domain,
		"orderers",
		fmt.Sprintf("%s.%s", o.Name, org.Domain),
		cryptoType,
	)
}

// OrdererLocalMSPDir returns the path to the local MSP directory for the
// Orderer.
func (n *Network) OrdererLocalMSPDir(o *topology.Orderer) string {
	return n.OrdererLocalCryptoDir(o, "msp")
}

// OrdererLocalTLSDir returns the path to the local TLS directory for the
// Orderer.
func (n *Network) OrdererLocalTLSDir(o *topology.Orderer) string {
	return n.OrdererLocalCryptoDir(o, "tls")
}

// ProfileForChannel gets the configtxgen profile name associated with the
// specified channel.
func (n *Network) ProfileForChannel(channelName string) string {
	for _, ch := range n.Channels {
		if ch.Name == channelName {
			return ch.Profile
		}
	}
	return ""
}

// CACertsBundlePath returns the path to the bundle of CA certificates for the
// network. This bundle is used when connecting to peers.
func (n *Network) CACertsBundlePath() string {
	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		"crypto",
		"ca-certs.pem",
	)
}

// bootstrapIdemix creates the idemix-related crypto material
func (n *Network) bootstrapIdemix() {
	for _, org := range n.IdemixOrgs() {
		output := n.IdemixOrgMSPDir(org)
		// - ca-keygen
		sess, err := n.Idemixgen(commands.CAKeyGen{
			NetworkPrefix: n.Prefix,
			Output:        output,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

func (n *Network) bootstrapExtraIdentities() {
	for i, peer := range n.Peers {
		for j, identity := range peer.Identities {
			switch identity.MSPType {
			case "idemix":
				org := n.Organization(identity.Org)
				output := n.IdemixOrgMSPDir(org)
				userOutput := filepath.Join(n.PeerLocalIdemixExtraIdentitiesDir(peer), identity.ID)
				sess, err := n.Idemixgen(commands.SignerConfig{
					NetworkPrefix:    n.Prefix,
					CAInput:          output,
					Output:           userOutput,
					OrgUnit:          org.Domain,
					EnrollmentID:     identity.EnrollmentID,
					RevocationHandle: fmt.Sprintf("1%d%d", i, j),
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
			case "bccsp":
				// Nothing to do here cause the extra identities are generated by crypto gen.
			default:
				Expect(identity.MSPType).To(Equal("idemix"))
			}
		}
	}
}

// ConcatenateTLSCACertificates concatenates all TLS CA certificates into a
// single file to be used by peer CLI.
func (n *Network) ConcatenateTLSCACertificates() {
	bundle := &bytes.Buffer{}
	for _, tlsCertPath := range n.ListTLSCACertificates() {
		certBytes, err := ioutil.ReadFile(tlsCertPath)
		Expect(err).NotTo(HaveOccurred())
		bundle.Write(certBytes)
	}
	if len(bundle.Bytes()) == 0 {
		return
	}

	err := ioutil.WriteFile(n.CACertsBundlePath(), bundle.Bytes(), 0660)
	Expect(err).NotTo(HaveOccurred())
}

// ListTLSCACertificates returns the paths of all TLS CA certificates in the
// network, across all organizations.
func (n *Network) ListTLSCACertificates() []string {
	fileName2Path := make(map[string]string)
	filepath.Walk(filepath.Join(n.Context.RootDir(), n.Prefix, "crypto"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// File starts with "tlsca" and has "-cert.pem" in it
		if strings.HasPrefix(info.Name(), "tlsca") && strings.Contains(info.Name(), "-cert.pem") {
			fileName2Path[info.Name()] = path
		}
		return nil
	})

	var tlsCACertificates []string
	for _, path := range fileName2Path {
		tlsCACertificates = append(tlsCACertificates, path)
	}
	return tlsCACertificates
}

// CreateAndJoinChannels will create all channels specified in the config that
// are referenced by peers. The referencing peers will then be joined to the
// channel(s).
//
// The network must be running before this is called.
func (n *Network) CreateAndJoinChannels(o *topology.Orderer) {
	for _, c := range n.Channels {
		n.CreateAndJoinChannel(o, c.Name)
	}
}

// CreateAndJoinChannel will create the specified channel. The referencing
// peers will then be joined to the channel.
//
// The network must be running before this is called.
func (n *Network) CreateAndJoinChannel(o *topology.Orderer, channelName string) {
	peers := n.PeersWithChannel(channelName)
	if len(peers) == 0 {
		return
	}

	n.CreateChannel(channelName, o, peers[0])
	n.JoinChannel(channelName, o, peers...)
}

// UpdateChannelAnchors determines the anchor peers for the specified channel,
// creates an anchor peer update transaction for each organization, and submits
// the update transactions to the orderer.
func (n *Network) UpdateChannelAnchors(o *topology.Orderer, channelName string) {
	tempFile, err := ioutil.TempFile("", "update-anchors")
	Expect(err).NotTo(HaveOccurred())
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	peersByOrg := map[string]*topology.Peer{}
	for _, p := range n.AnchorsForChannel(channelName) {
		peersByOrg[p.Organization] = p
	}

	for orgName, p := range peersByOrg {
		anchorUpdate := commands.OutputAnchorPeersUpdate{
			NetworkPrefix:           n.Prefix,
			OutputAnchorPeersUpdate: tempFile.Name(),
			ChannelID:               channelName,
			Profile:                 n.ProfileForChannel(channelName),
			ConfigPath:              filepath.Join(n.Context.RootDir(), n.Prefix),
			AsOrg:                   orgName,
		}
		sess, err := n.ConfigTxGen(anchorUpdate)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		sess, err = n.PeerAdminSession(p, commands.ChannelUpdate{
			NetworkPrefix: n.Prefix,
			ChannelID:     channelName,
			Orderer:       n.OrdererAddress(o, ListenPort),
			File:          tempFile.Name(),
			ClientAuth:    n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

// VerifyMembership checks that each peer has discovered the expected
// peers in the network
func (n *Network) VerifyMembership(expectedPeers []*topology.Peer, channel string, chaincodes ...string) {
	// all peers currently include _lifecycle as an available chaincode
	chaincodes = append(chaincodes, "_lifecycle")
	expectedDiscoveredPeerMatchers := make([]types.GomegaMatcher, len(expectedPeers))
	for i, peer := range expectedPeers {
		expectedDiscoveredPeerMatchers[i] = n.DiscoveredPeerMatcher(peer, chaincodes...) // n.DiscoveredPeer(peer, chaincodes...)
	}
	for _, peer := range expectedPeers {
		Eventually(DiscoverPeers(n, peer, "User1", channel), n.EventuallyTimeout).Should(ConsistOf(expectedDiscoveredPeerMatchers))
	}
}

// CreateChannel will submit an existing create channel transaction to the
// specified orderer. The channel transaction must exist at the location
// returned by CreateChannelTxPath.  Optionally, additional signers may be
// included in the case where the channel creation tx modifies other
// aspects of the channel config for the new channel.
//
// The orderer must be running when this is called.
func (n *Network) CreateChannel(channelName string, o *topology.Orderer, p *topology.Peer, additionalSigners ...interface{}) {
	channelCreateTxPath := n.CreateChannelTxPath(channelName)
	n.signConfigTransaction(channelCreateTxPath, p, additionalSigners...)

	createChannel := func() int {
		sess, err := n.PeerAdminSession(p, commands.ChannelCreate{
			NetworkPrefix: n.Prefix,
			ChannelID:     channelName,
			Orderer:       n.OrdererAddress(o, ListenPort),
			File:          channelCreateTxPath,
			OutputBlock:   "/dev/null",
			ClientAuth:    n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		return sess.Wait(n.EventuallyTimeout).ExitCode()
	}
	Eventually(createChannel, n.EventuallyTimeout).Should(Equal(0))
}

// CreateChannelExitCode will submit an existing create channel transaction to
// the specified orderer, wait for the operation to complete, and return the
// exit status of the "peer channel create" command.
//
// The channel transaction must exist at the location returned by
// CreateChannelTxPath and the orderer must be running when this is called.
func (n *Network) CreateChannelExitCode(channelName string, o *topology.Orderer, p *topology.Peer, additionalSigners ...interface{}) int {
	channelCreateTxPath := n.CreateChannelTxPath(channelName)
	n.signConfigTransaction(channelCreateTxPath, p, additionalSigners...)

	sess, err := n.PeerAdminSession(p, commands.ChannelCreate{
		NetworkPrefix: n.Prefix,
		ChannelID:     channelName,
		Orderer:       n.OrdererAddress(o, ListenPort),
		File:          channelCreateTxPath,
		OutputBlock:   "/dev/null",
		ClientAuth:    n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	return sess.Wait(n.EventuallyTimeout).ExitCode()
}

func (n *Network) signConfigTransaction(channelTxPath string, submittingPeer *topology.Peer, signers ...interface{}) {
	for _, signer := range signers {
		switch signer := signer.(type) {
		case *topology.Peer:
			sess, err := n.PeerAdminSession(signer, commands.SignConfigTx{
				NetworkPrefix: n.Prefix,
				File:          channelTxPath,
				ClientAuth:    n.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		case *topology.Orderer:
			sess, err := n.OrdererAdminSession(signer, submittingPeer, commands.SignConfigTx{
				NetworkPrefix: n.Prefix,
				File:          channelTxPath,
				ClientAuth:    n.ClientAuthRequired,
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		default:
			panic(fmt.Sprintf("unknown signer type %T, expect Peer or Orderer", signer))
		}
	}
}

// JoinChannel will join peers to the specified channel. The orderer is used to
// obtain the current configuration block for the channel.
//
// The orderer and listed peers must be running before this is called.
func (n *Network) JoinChannel(name string, o *topology.Orderer, peers ...*topology.Peer) {
	if len(peers) == 0 {
		return
	}

	tempFile, err := ioutil.TempFile("", "genesis-block")
	Expect(err).NotTo(HaveOccurred())
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	sess, err := n.PeerAdminSession(peers[0], commands.ChannelFetch{
		NetworkPrefix: n.Prefix,
		Block:         "0",
		ChannelID:     name,
		Orderer:       n.OrdererAddress(o, ListenPort),
		OutputFile:    tempFile.Name(),
		ClientAuth:    n.ClientAuthRequired,
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

	for _, p := range peers {
		if p.SkipInit {
			continue
		}
		sess, err := n.PeerAdminSession(p, commands.ChannelJoin{
			NetworkPrefix: n.Prefix,
			BlockPath:     tempFile.Name(),
			ClientAuth:    n.ClientAuthRequired,
		})
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
}

// Cryptogen starts a gexec.Session for the provided cryptogen command.
func (n *Network) Cryptogen(command common.Command) (*gexec.Session, error) {
	// note that we will not use the cryptogen tool provided by fabric;
	// thus we build our own
	cmd := common.NewCommand(n.Builder.FSCCLI(), command)
	return n.StartSession(cmd, command.SessionName())
}

// Idemixgen starts a gexec.Session for the provided idemixgen command.
func (n *Network) Idemixgen(command common.Command) (*gexec.Session, error) {
	cmdPath := findOrBuild(idemixgenCMD, n.Builder.Idemixgen)
	cmd := common.NewCommand(cmdPath, command)
	return n.StartSession(cmd, command.SessionName())
}

// ConfigTxGen starts a gexec.Session for the provided configtxgen command.
func (n *Network) ConfigTxGen(command common.Command) (*gexec.Session, error) {
	cmdPath := findCmdAtEnv(configtxgenCMD)
	Expect(cmdPath).NotTo(Equal(""), "could not find %s in %s directory %s", configtxgenCMD, FabricBinsPathEnvKey, os.Getenv(FabricBinsPathEnvKey))

	cmd := common.NewCommand(cmdPath, command)
	return n.StartSession(cmd, command.SessionName())
}

// Discover starts a gexec.Session for the provided discover command.
func (n *Network) Discover(command common.Command) (*gexec.Session, error) {
	cmdPath := findCmdAtEnv(discoverCMD)
	Expect(cmdPath).NotTo(Equal(""), "could not find %s in %s directory %s", configtxgenCMD, FabricBinsPathEnvKey, os.Getenv(FabricBinsPathEnvKey))

	cmd := common.NewCommand(cmdPath, command)
	cmd.Args = append(cmd.Args, "--peerTLSCA", n.CACertsBundlePath())
	return n.StartSession(cmd, command.SessionName())
}

// OrdererRunner returns an ifrit.Runner for the specified orderer. The runner
// can be used to start and manage an orderer process.
func (n *Network) OrdererRunner(o *topology.Orderer) *runner2.Runner {
	cmdPath := findCmdAtEnv(ordererCMD)
	Expect(cmdPath).NotTo(Equal(""), "could not find %s in %s directory %s", configtxgenCMD, FabricBinsPathEnvKey, os.Getenv(FabricBinsPathEnvKey))

	cmd := exec.Command(cmdPath)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("FABRIC_CFG_PATH=%s", n.OrdererDir(o)))
	cmd.Env = append(cmd.Env, "FABRIC_LOGGING_SPEC="+n.Logging.Spec)

	config := runner2.Config{
		AnsiColorCode:     n.nextColor(),
		Name:              n.Prefix + "-" + o.ID(),
		Command:           cmd,
		StartCheck:        "Beginning to serve requests",
		StartCheckTimeout: 1 * time.Minute,
	}

	if n.Topology().LogOrderersToFile {
		// set stdout to a file
		Expect(os.MkdirAll(n.OrdererLogsFolder(), 0755)).ToNot(HaveOccurred())
		f, err := os.Create(
			filepath.Join(
				n.OrdererLogsFolder(),
				fmt.Sprintf("%s-%s.log", o.Name, n.Organization(o.Organization).Domain),
			),
		)
		Expect(err).ToNot(HaveOccurred())
		config.Stdout = f
		config.Stderr = f
	}

	return runner2.New(config)
}

// OrdererGroupRunner returns a runner that can be used to start and stop all
// orderers in a network.
func (n *Network) OrdererGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	for _, o := range n.Orderers {
		members = append(members, grouper.Member{Name: o.ID(), Runner: n.OrdererRunner(o)})
	}
	if len(members) == 0 {
		return nil
	}

	return grouper.NewParallel(syscall.SIGTERM, members)
}

// PeerRunner returns an ifrit.Runner for the specified peer. The runner can be
// used to start and manage a peer process.
func (n *Network) PeerRunner(p *topology.Peer, env ...string) *runner2.Runner {
	cmd := n.peerCommand(
		p.ExecutablePath,
		commands.NodeStart{
			NetworkPrefix: n.Prefix,
			PeerID:        p.ID(),
			DevMode:       p.DevMode,
		},
		"",
		fmt.Sprintf("FABRIC_CFG_PATH=%s", n.PeerDir(p)),
		fmt.Sprintf("CORE_PEER_ID=%s", fmt.Sprintf("%s.%s", p.Name, n.Organization(p.Organization).Domain)),
	)

	cmd.Env = append(cmd.Env, env...)
	//cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	config := runner2.Config{
		AnsiColorCode:     n.nextColor(),
		Name:              n.Prefix + "-" + p.ID(),
		Command:           cmd,
		StartCheck:        `Started peer with ID=.*, address=`,
		StartCheckTimeout: 1 * time.Minute,
	}
	if n.Topology().LogPeersToFile {
		// set stdout to a file
		Expect(os.MkdirAll(n.PeerLogsFolder(), 0755)).ToNot(HaveOccurred())
		f, err := os.Create(
			filepath.Join(
				n.PeerLogsFolder(),
				fmt.Sprintf("%s-%s.log", p.Name, n.Organization(p.Organization).Domain),
			),
		)
		Expect(err).ToNot(HaveOccurred())
		config.Stdout = f
		config.Stderr = f
	}

	return runner2.New(config)
}

func (n *Network) PeerLogsFolder() string {
	return filepath.Join(
		n.RootDir,
		n.Prefix,
		"logs",
		"peers",
	)
}

func (n *Network) OrdererLogsFolder() string {
	return filepath.Join(
		n.RootDir,
		n.Prefix,
		"logs",
		"orderers",
	)
}

// PeerGroupRunner returns a runner that can be used to start and stop all
// peers in a network.
func (n *Network) PeerGroupRunner() ifrit.Runner {
	members := grouper.Members{}
	for _, p := range n.Peers {
		if p.SkipRunning {
			continue
		}
		switch {
		case p.Type == topology.FabricPeer:
			members = append(members, grouper.Member{Name: p.ID(), Runner: n.PeerRunner(p)})
		}
	}
	if len(members) == 0 {
		return nil
	}
	return grouper.NewParallel(syscall.SIGTERM, members)
}

func (n *Network) peerCommand(executablePath string, command common.Command, tlsDir string, env ...string) *exec.Cmd {
	cmdPath := findCmdAtEnv(peerCMD)
	Expect(cmdPath).NotTo(Equal(""), "could not find %s in %s directory %s", configtxgenCMD, FabricBinsPathEnvKey, os.Getenv(FabricBinsPathEnvKey))
	logger.Debugf("Found %s => %s", peerCMD, cmdPath)

	cmd := common.NewCommand(cmdPath, command)
	cmd.Env = append(cmd.Env, env...)
	cmd.Env = append(cmd.Env, "FABRIC_LOGGING_SPEC="+n.Logging.Spec)

	if n.GRPCLogging {
		cmd.Env = append(cmd.Env, "GRPC_GO_LOG_VERBOSITY_LEVEL=2")
		cmd.Env = append(cmd.Env, "GRPC_GO_LOG_SEVERITY_LEVEL=debug")
	}

	if common.ConnectsToOrderer(command) {
		cmd.Args = append(cmd.Args, "--tls")
		cmd.Args = append(cmd.Args, "--cafile", n.CACertsBundlePath())
	}

	if common.ClientAuthEnabled(command) {
		certfilePath := filepath.Join(tlsDir, "client.crt")
		keyfilePath := filepath.Join(tlsDir, "client.key")

		cmd.Args = append(cmd.Args, "--certfile", certfilePath)
		cmd.Args = append(cmd.Args, "--keyfile", keyfilePath)
	}

	cmd.Args = append(cmd.Args, "--logging-level", n.Logging.Spec)

	// In case we have a peer invoke with multiple certificates,
	// we need to mimic the correct peer CLI usage,
	// so we count the number of --peerAddresses usages
	// we have, and add the same (concatenated TLS CA certificates file)
	// the same number of times to bypass the peer CLI sanity checks
	requiredPeerAddresses := flagCount("--peerAddresses", cmd.Args)
	for i := 0; i < requiredPeerAddresses; i++ {
		cmd.Args = append(cmd.Args, "--tlsRootCertFiles")
		cmd.Args = append(cmd.Args, n.CACertsBundlePath())
	}
	return cmd
}

func flagCount(flag string, args []string) int {
	var c int
	for _, arg := range args {
		if arg == flag {
			c++
		}
	}
	return c
}

// PeerAdminSession starts a gexec.Session as a peer admin for the provided
// peer command. This is intended to be used by short running peer fsccli commands
// that execute in the context of a peer configuration.
func (n *Network) PeerAdminSession(p *topology.Peer, command common.Command) (*gexec.Session, error) {
	return n.PeerUserSession(p, "Admin", command)
}

// PeerUserSession starts a gexec.Session as a peer user for the provided peer
// command. This is intended to be used by short running peer fsccli commands that
// execute in the context of a peer configuration.
func (n *Network) PeerUserSession(p *topology.Peer, user string, command common.Command) (*gexec.Session, error) {
	cmd := n.peerCommand(
		p.ExecutablePath,
		command,
		n.PeerUserTLSDir(p, user),
		fmt.Sprintf("FABRIC_CFG_PATH=%s", n.PeerDir(p)),
		fmt.Sprintf("CORE_PEER_MSPCONFIGPATH=%s", n.PeerUserMSPDir(p, user)),
	)
	return n.StartSession(cmd, command.SessionName())
}

// OrdererAdminSession starts a gexec.Session as an orderer admin user. This
// is used primarily to generate orderer configuration updates.
func (n *Network) OrdererAdminSession(o *topology.Orderer, p *topology.Peer, command common.Command) (*gexec.Session, error) {
	cmd := n.peerCommand(
		p.ExecutablePath,
		command,
		n.ordererUserCryptoDir(o, "Admin", "tls"),
		fmt.Sprintf("CORE_PEER_LOCALMSPID=%s", n.Organization(o.Organization).MSPID),
		fmt.Sprintf("FABRIC_CFG_PATH=%s", n.PeerDir(p)),
		fmt.Sprintf("CORE_PEER_MSPCONFIGPATH=%s", n.OrdererUserMSPDir(o, "Admin")),
	)

	return n.StartSession(cmd, command.SessionName())
}

// Peer returns the information about the named Peer in the named organization.
func (n *Network) Peer(orgName, peerName string) *topology.Peer {
	for _, p := range n.PeersInOrg(orgName) {
		if p.Name == peerName {
			return p
		}
	}
	return nil
}

// DiscoveredPeer creates a new DiscoveredPeer from the peer and chaincodes
// passed as arguments.
func (n *Network) DiscoveredPeer(p *topology.Peer, chaincodes ...string) DiscoveredPeer {
	peerCert, err := ioutil.ReadFile(n.PeerCert(p))
	Expect(err).NotTo(HaveOccurred())

	return DiscoveredPeer{
		MSPID:      n.Organization(p.Organization).MSPID,
		Endpoint:   n.PeerAddress(p, ListenPort),
		Identity:   string(peerCert),
		Chaincodes: chaincodes,
	}
}

func (n *Network) DiscoveredPeerMatcher(p *topology.Peer, chaincodes ...string) types.GomegaMatcher {
	peerCert, err := ioutil.ReadFile(n.PeerCert(p))
	Expect(err).NotTo(HaveOccurred())

	return gstruct.MatchAllFields(gstruct.Fields{
		"MSPID":      Equal(n.Organization(p.Organization).MSPID),
		"Endpoint":   Equal(n.PeerAddress(p, ListenPort)),
		"Identity":   Equal(string(peerCert)),
		"Chaincodes": containElements(chaincodes...),
	})
}

// containElements succeeds if a slice contains the passed in elements.
func containElements(elements ...string) types.GomegaMatcher {
	ms := make([]types.GomegaMatcher, 0, len(elements))
	for _, element := range elements {
		ms = append(ms, &matchers.ContainElementMatcher{
			Element: element,
		})
	}
	return &matchers.AndMatcher{
		Matchers: ms,
	}
}

// Orderer returns the information about the named Orderer.
func (n *Network) Orderer(name string) *topology.Orderer {
	for _, o := range n.Orderers {
		if o.Name == name {
			return o
		}
	}
	return nil
}

// Organization returns the information about the named Organization.
func (n *Network) Organization(orgName string) *topology.Organization {
	for _, org := range n.Organizations {
		if org.Name == orgName {
			return org
		}
	}
	return nil
}

// Consortium returns information about the named Consortium.
func (n *Network) Consortium(name string) *topology.Consortium {
	for _, c := range n.Consortiums {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// PeerOrgs returns all Organizations associated with at least one Peer.
func (n *Network) PeerOrgs() []*topology.Organization {
	orgsByName := map[string]*topology.Organization{}
	for _, p := range n.Peers {
		if n.Organization(p.Organization).MSPType != "idemix" {
			orgsByName[p.Organization] = n.Organization(p.Organization)
		}
	}

	var orgs []*topology.Organization
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

func (n *Network) PeerOrgsByPeers(peers []*topology.Peer) []*topology.Organization {
	orgsByName := map[string]*topology.Organization{}
	for _, p := range peers {
		if n.Organization(p.Organization).MSPType != "idemix" {
			orgsByName[p.Organization] = n.Organization(p.Organization)
		}
	}

	orgs := []*topology.Organization{}
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

// IdemixOrgs returns all Organizations of type idemix.
func (n *Network) IdemixOrgs() []*topology.Organization {
	orgs := []*topology.Organization{}
	for _, org := range n.Organizations {
		if org.MSPType == "idemix" {
			orgs = append(orgs, org)
		}
	}
	return orgs
}

// PeersWithChannel returns all Peer instances that have joined the named
// channel.
func (n *Network) PeersWithChannel(chanName string) []*topology.Peer {
	var peers []*topology.Peer
	for _, p := range n.Peers {
		switch {
		case p.Type == topology.FabricPeer:
			for _, c := range p.Channels {
				if c.Name == chanName {
					peers = append(peers, p)
				}
			}
		}
	}

	// This is a bit of a hack to make the output of this function deterministic.
	// When this function's output is supplied as input to functions such as ApproveChaincodeForMyOrg
	// it causes a different subset of peers to be picked, which can create flakiness in tests.
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].Organization < peers[j].Organization {
			return true
		}

		return peers[i].Organization == peers[j].Organization && peers[i].Name < peers[j].Name
	})
	return peers
}

// AnchorsForChannel returns all Peer instances that are anchors for the
// named channel.
func (n *Network) AnchorsForChannel(chanName string) []*topology.Peer {
	anchors := []*topology.Peer{}
	for _, p := range n.Peers {
		switch {
		case p.Type == topology.FabricPeer:
			for _, pc := range p.Channels {
				if pc.Name == chanName && pc.Anchor {
					anchors = append(anchors, p)
				}
			}
		}
	}
	return anchors
}

// AnchorsInOrg returns all peers that are an anchor for at least one channel
// in the named organization.
func (n *Network) AnchorsInOrg(orgName string) []*topology.Peer {
	anchors := []*topology.Peer{}
	for _, p := range n.PeersInOrg(orgName) {
		if p.Anchor() {
			anchors = append(anchors, p)
			break
		}
	}

	// No explicit anchor means all peers are anchors.
	if len(anchors) == 0 {
		anchors = n.PeersInOrg(orgName)
	}

	return anchors
}

// OrderersInOrg returns all Orderer instances owned by the named organaiztion.
func (n *Network) OrderersInOrg(orgName string) []*topology.Orderer {
	orderers := []*topology.Orderer{}
	for _, o := range n.Orderers {
		if o.Organization == orgName {
			orderers = append(orderers, o)
		}
	}
	return orderers
}

// OrgsForOrderers returns all Organization instances that own at least one of
// the named orderers.
func (n *Network) OrgsForOrderers(ordererNames []string) []*topology.Organization {
	orgsByName := map[string]*topology.Organization{}
	for _, name := range ordererNames {
		orgName := n.Orderer(name).Organization
		orgsByName[orgName] = n.Organization(orgName)
	}
	orgs := []*topology.Organization{}
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

// OrdererOrgs returns all Organization instances that own at least one
// orderer.
func (n *Network) OrdererOrgs() []*topology.Organization {
	orgsByName := map[string]*topology.Organization{}
	for _, o := range n.Orderers {
		orgsByName[o.Organization] = n.Organization(o.Organization)
	}

	orgs := []*topology.Organization{}
	for _, org := range orgsByName {
		orgs = append(orgs, org)
	}
	return orgs
}

func (n *Network) PeersInOrgWithOptions(orgName string, includeAll bool) []*topology.Peer {
	var peers []*topology.Peer
	for _, o := range n.Peers {
		if o.Type != topology.FabricPeer && !includeAll {
			continue
		}
		if o.Organization == orgName {
			peers = append(peers, o)
		}
	}
	return peers
}

// PeersInOrg returns all Peer instances that are owned by the named
// organization.
func (n *Network) PeersInOrg(orgName string) []*topology.Peer {
	return n.PeersInOrgWithOptions(orgName, true)
}

const (
	ChaincodePort  api.PortName = "Chaincode"
	EventsPort     api.PortName = "Events"
	HostPort       api.PortName = "HostPort"
	ListenPort     api.PortName = "Listen"
	ProfilePort    api.PortName = "Profile"
	OperationsPort api.PortName = "Operations"
	ViewPort       api.PortName = "View"
	P2PPort        api.PortName = "P2P"
	ClusterPort    api.PortName = "Cluster"
	WebPort        api.PortName = "Web"
)

// PeerPortNames returns the list of ports that need to be reserved for a Peer.
func PeerPortNames() []api.PortName {
	return []api.PortName{ListenPort, ChaincodePort, EventsPort, ProfilePort, OperationsPort, P2PPort, WebPort}
}

// OrdererPortNames  returns the list of ports that need to be reserved for an
// Orderer.
func OrdererPortNames() []api.PortName {
	return []api.PortName{ListenPort, ProfilePort, OperationsPort, ClusterPort}
}

// OrdererAddress returns the address (host and port) exposed by the Orderer
// for the named port. Commands line tools should use the returned address when
// connecting to the orderer.
//
// This assumes that the orderer is listening on 0.0.0.0 or 127.0.0.1 and is
// available on the loopback address.
func (n *Network) OrdererAddress(o *topology.Orderer, portName api.PortName) string {
	return fmt.Sprintf("%s:%d", n.OrdererHost(o), n.OrdererPort(o, portName))
}

// OrdererPort returns the named port reserved for the Orderer instance.
func (n *Network) OrdererPort(o *topology.Orderer, portName api.PortName) uint16 {
	ordererPorts := n.Context.PortsByOrdererID(n.Prefix, o.ID())
	Expect(ordererPorts).NotTo(BeNil(), "expected orderer ports to be initialized [%s]", o.ID())
	return ordererPorts[portName]
}

// OrdererHost returns the hostname of the Orderer instance.
func (n *Network) OrdererHost(o *topology.Orderer) string {
	ordererHost := n.Context.HostByOrdererID(n.Prefix, o.ID())
	Expect(ordererHost).NotTo(BeNil(), "expected orderer host to be initialized [%s]", o.ID())
	return ordererHost
}

// PeerAddress returns the address (host and port) exposed by the Peer for the
// named port. Commands line tools should use the returned address when
// connecting to a peer.
//
// This assumes that the peer is listening on 0.0.0.0 and is available on the
// loopback address.
func (n *Network) PeerAddress(p *topology.Peer, portName api.PortName) string {
	if p.Hostname != "" {
		return fmt.Sprintf("%s:%d", p.Hostname, n.PeerPort(p, portName))
	}
	return fmt.Sprintf("%s:%d", n.PeerHost(p), n.PeerPort(p, portName))
}

func (n *Network) PeerAddressByName(p *topology.Peer, portName api.PortName) string {
	return fmt.Sprintf("%s:%d", n.PeerHost(p), n.PeerPortByName(p, portName))
}

// PeerPort returns the named port reserved for the Peer instance.
func (n *Network) PeerPort(p *topology.Peer, portName api.PortName) uint16 {
	peerPorts := n.Context.PortsByPeerID(n.Prefix, p.ID())
	if peerPorts == nil {
		fmt.Printf("PeerPort [%s,%s] not found", p.ID(), portName)
	}
	Expect(peerPorts).NotTo(BeNil(), "PeerPort [%s,%s] not found", p.ID(), portName)
	return peerPorts[portName]
}

// PeerHost returns the hostname of the Peer instance.
func (n *Network) PeerHost(o *topology.Peer) string {
	peerHost := n.Context.HostByPeerID(n.Prefix, o.ID())
	Expect(peerHost).NotTo(BeNil(), "expected host host to be initialized [%s]", o.ID())
	return peerHost
}

func (n *Network) PeerPortByName(p *topology.Peer, portName api.PortName) uint16 {
	peerPorts := n.Context.PortsByPeerID(n.Prefix, p.Name)
	Expect(peerPorts).NotTo(BeNil())
	return peerPorts[portName]
}

func (n *Network) BootstrapNode(me *topology.Peer) string {
	for _, p := range n.Peers {
		if p.Bootstrap {
			if p.Name == me.Name {
				return ""
			}
			return p.Name
		}
	}
	return ""
}

func (n *Network) nextColor() string {
	color := n.colorIndex%14 + 31
	if color > 37 {
		color = color + 90 - 37
	}

	n.colorIndex++
	return fmt.Sprintf("%dm", color)
}

func (n *Network) FSCNodeVaultDir(peer *topology.Peer) string {
	return filepath.Join(n.Context.RootDir(), "fsc", "nodes", peer.Name, n.Prefix, "vault")
}

func (n *Network) OrdererBootstrapFile() string {
	return filepath.Join(
		n.Context.RootDir(),
		n.Prefix,
		n.SystemChannel.Name+"_block.pb",
	)
}

// StartSession executes a command session. This should be used to launch
// command line tools that are expected to run to completion.
func (n *Network) StartSession(cmd *exec.Cmd, name string) (*gexec.Session, error) {
	ansiColorCode := n.nextColor()
	fmt.Fprintf(
		ginkgo.GinkgoWriter,
		"\x1b[33m[d]\x1b[%s[%s]\x1b[0m starting %s %s\n",
		ansiColorCode,
		name,
		filepath.Base(cmd.Args[0]),
		strings.Join(cmd.Args[1:], " "),
	)
	return gexec.Start(
		cmd,
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", ansiColorCode, name),
			ginkgo.GinkgoWriter,
		),
		gexec.NewPrefixedWriter(
			fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", ansiColorCode, name),
			ginkgo.GinkgoWriter,
		),
	)
}

func (n *Network) GenerateCryptoConfig() {
	Expect(os.MkdirAll(n.CryptoPath(), 0770)).NotTo(HaveOccurred())
	crypto, err := os.Create(n.CryptoConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer crypto.Close()

	t, err := template.New("crypto").Parse(n.Templates.CryptoTemplate())
	Expect(err).NotTo(HaveOccurred())

	// pw := gexec.NewPrefixedWriter("[crypto-config.yaml] ", ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(crypto), n)
	Expect(err).NotTo(HaveOccurred())
}

func (n *Network) GenerateConfigTxConfig() {
	config, err := os.Create(n.ConfigTxConfigPath())
	Expect(err).NotTo(HaveOccurred())
	defer config.Close()

	t, err := template.New("configtx").Parse(n.Templates.ConfigTxTemplate())
	Expect(err).NotTo(HaveOccurred())

	// pw := gexec.NewPrefixedWriter("[configtx.yaml] ", ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(config), n)
	Expect(err).NotTo(HaveOccurred())
}

func (n *Network) GenerateOrdererConfig(o *topology.Orderer) {
	err := os.MkdirAll(n.OrdererDir(o), 0755)
	Expect(err).NotTo(HaveOccurred())

	orderer, err := os.Create(n.OrdererConfigPath(o))
	Expect(err).NotTo(HaveOccurred())
	defer orderer.Close()

	t, err := template.New("orderer").Funcs(template.FuncMap{
		"Orderer":    func() *topology.Orderer { return o },
		"ToLower":    func(s string) string { return strings.ToLower(s) },
		"ReplaceAll": func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
	}).Parse(n.Templates.OrdererTemplate())
	Expect(err).NotTo(HaveOccurred())

	// pw := gexec.NewPrefixedWriter(fmt.Sprintf("[%s#orderer.yaml] ", o.ID()), ginkgo.GinkgoWriter)
	err = t.Execute(io.MultiWriter(orderer), n)
	Expect(err).NotTo(HaveOccurred())
}

func (n *Network) GenerateCoreConfig(p *topology.Peer) {
	switch p.Type {
	case topology.FabricPeer:
		err := os.MkdirAll(n.PeerDir(p), 0755)
		Expect(err).NotTo(HaveOccurred())

		core, err := os.Create(n.PeerConfigPath(p))
		Expect(err).NotTo(HaveOccurred())
		defer core.Close()

		coreTemplate := n.Templates.CoreTemplate()

		t, err := template.New("peer").Funcs(template.FuncMap{
			"Peer":                      func() *topology.Peer { return p },
			"Orderer":                   func() *topology.Orderer { return n.Orderers[0] },
			"PeerLocalExtraIdentityDir": func(p *topology.Peer, id string) string { return n.PeerLocalExtraIdentityDir(p, id) },
			"ToLower":                   func(s string) string { return strings.ToLower(s) },
			"ReplaceAll":                func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
		}).Parse(coreTemplate)
		Expect(err).NotTo(HaveOccurred())

		extension := bytes.NewBuffer([]byte{})
		err = t.Execute(io.MultiWriter(core, extension), n)
		Expect(err).NotTo(HaveOccurred())
		n.Context.AddExtension(p.ID(), api.FabricExtension, extension.String())
	case topology.FSCPeer:
		err := os.MkdirAll(n.PeerDir(p), 0755)
		Expect(err).NotTo(HaveOccurred())

		var refPeers []*topology.Peer
		coreTemplate := n.Templates.CoreTemplate()
		defaultNetwork := n.topology.Default
		driver := n.topology.Driver
		tlsEnabled := n.topology.TLSEnabled
		if p.Type == topology.FSCPeer {
			coreTemplate = n.Templates.FSCFabricExtensionTemplate()
			peers := n.PeersInOrg(p.Organization)
			defaultNetwork = p.DefaultNetwork
			for _, peer := range peers {
				if peer.Type == topology.FabricPeer {
					refPeers = append(refPeers, peer)
				}
			}
		}
		for _, identity := range p.Identities {
			if len(identity.Path) == 0 {
				identity.Path = n.PeerLocalExtraIdentityDir(p, identity.ID)
			}
		}

		t, err := template.New("peer").Funcs(template.FuncMap{
			"Peer":                        func() *topology.Peer { return p },
			"Orderers":                    func() []*topology.Orderer { return n.Orderers },
			"PeerLocalExtraIdentityDir":   func(p *topology.Peer, id string) string { return n.PeerLocalExtraIdentityDir(p, id) },
			"ToLower":                     func(s string) string { return strings.ToLower(s) },
			"ReplaceAll":                  func(s, old, new string) string { return strings.Replace(s, old, new, -1) },
			"Peers":                       func() []*topology.Peer { return refPeers },
			"OrdererAddress":              func(o *topology.Orderer, portName api.PortName) string { return n.OrdererAddress(o, portName) },
			"PeerAddress":                 func(o *topology.Peer, portName api.PortName) string { return n.PeerAddress(o, portName) },
			"CACertsBundlePath":           func() string { return n.CACertsBundlePath() },
			"FSCNodeVaultPersistenceType": func() string { return GetPersistenceType(p) },
			"FSCNodeVaultOrionNetwork":    func() string { return GetVaultPersistenceOrionNetwork(p) },
			"FSCNodeVaultOrionDatabase":   func() string { return GetVaultPersistenceOrionDatabase(p) },
			"FSCNodeVaultOrionCreator":    func() string { return GetVaultPersistenceOrionCreator(p) },
			"FSCNodeVaultPath":            func() string { return n.FSCNodeVaultDir(p) },
			"FabricName":                  func() string { return n.topology.Name() },
			"DefaultNetwork":              func() bool { return defaultNetwork },
			"Driver":                      func() string { return driver },
			"Chaincodes":                  func(channel string) []*topology.ChannelChaincode { return n.Chaincodes(channel) },
			"TLSEnabled":                  func() bool { return tlsEnabled },
		}).Parse(coreTemplate)
		Expect(err).NotTo(HaveOccurred())

		extension := bytes.NewBuffer([]byte{})
		err = t.Execute(io.MultiWriter(extension), n)
		Expect(err).NotTo(HaveOccurred())
		n.Context.AddExtension(p.Name, api.FabricExtension, extension.String())
	}
}

func (n *Network) PeersByName(names []string) []*topology.Peer {
	var peers []*topology.Peer
	for _, p := range n.Peers {
		for _, name := range names {
			if p.Name == name {
				peers = append(peers, p)
				break
			}
		}
	}
	return peers
}

func (n *Network) PeersForChaincodeByName(names []string) []*topology.Peer {
	var peers []*topology.Peer
	for _, p := range n.Peers {
		if p.SkipInit {
			continue
		}
		for _, name := range names {
			if p.Name == name {
				peers = append(peers, p)
				break
			}
		}
	}
	return peers
}

func (n *Network) PeerByName(name string) *topology.Peer {
	for _, p := range n.Peers {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (n *Network) FSCPeerByName(name string) *topology.Peer {
	for _, p := range n.Peers {
		if p.Name == name && p.Type == topology.FSCPeer {
			return p
		}
	}
	return nil
}

func (n *Network) Chaincodes(channel string) []*topology.ChannelChaincode {
	var res []*topology.ChannelChaincode
	for _, chaincode := range n.topology.Chaincodes {
		if chaincode.Channel == channel {
			res = append(res, chaincode)
		}
	}
	return res
}

func (n *Network) AppendPeer(peer *topology.Peer) {
	n.Peers = append(n.Peers, peer)
}

func GetLinkedIdentities(peer *topology.Peer) []string {
	v := peer.FSCNode.Options.Get("fabric.linked.identities")
	if v == nil {
		return nil
	}
	boxed, ok := v.([]interface{})
	if ok {
		var res []string
		for _, b := range boxed {
			res = append(res, b.(string))
		}
		return res
	}
	return v.([]string)
}

func GetPersistenceType(peer *topology.Peer) string {
	v := peer.FSCNode.Options.Get("fabric.vault.persistence.orion")
	if v == nil {
		return "badger"
	}
	return "orion"
}

func GetVaultPersistenceOrionNetwork(peer *topology.Peer) string {
	v := peer.FSCNode.Options.Get("fabric.vault.persistence.orion")
	Expect(v).NotTo(BeNil())
	return v.(string)
}

func GetVaultPersistenceOrionDatabase(peer *topology.Peer) string {
	v := peer.FSCNode.Options.Get("fabric.vault.persistence.orion.database")
	Expect(v).NotTo(BeNil())
	return v.(string)
}

func GetVaultPersistenceOrionCreator(peer *topology.Peer) string {
	v := peer.FSCNode.Options.Get("fabric.vault.persistence.orion.creator")
	Expect(v).NotTo(BeNil())
	return v.(string)
}

// BCCSPOpts returns a `topology.BCCSP` instance. `defaultProvider` sets the `Default` value of the BCCSP,
// that is denoting the which provider impl is used. `defaultProvider` currently supports `SW` and `PKCS11`.
func BCCSPOpts(defaultProvider string) *topology.BCCSP {
	bccsp := &topology.BCCSP{
		Default: defaultProvider,
		SW: &topology.SoftwareProvider{
			Hash:     "SHA2",
			Security: 256,
		},
		PKCS11: &topology.PKCS11{
			Hash:     "SHA2",
			Security: 256,
		},
	}
	if defaultProvider == pkcs11.Provider {
		lib, pin, label, err := pkcs11.FindPKCS11Lib()
		Expect(err).ToNot(HaveOccurred(), "cannot find PKCS11 lib")
		bccsp.PKCS11.Pin = pin
		bccsp.PKCS11.Label = label
		bccsp.PKCS11.Library = lib
	}
	return bccsp
}
