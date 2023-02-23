/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/fpc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger/fabric-private-chaincode/client_sdk/go/pkg/core/contract"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit/grouper"
)

var logger = flogging.MustGetLogger("nwo.fabric")

type Orderer struct {
	Name             string
	FullName         string
	ListeningAddress string
	TLSCACerts       []string
}

type Identity struct {
	ID    string
	Type  string
	Path  string
	MSPID string
}

type Peer struct {
	Name             string
	FullName         string
	ListeningAddress string
	TLSCACerts       []string
	Cert             string
	Identities       []*Identity
}

type Org struct {
	Name                  string
	MSPID                 string
	CACertsBundlePath     string
	PeerCACertificatePath string
}

type User struct {
	Name string
	Cert string
	Key  string
}

type Chaincode struct {
	Name      string
	OrgMSPIDs []string
}

type Channel struct {
	Name       string
	Chaincodes []*Chaincode
}

type BuilderClient interface {
	Build(path string) string
}

type platformFactory struct{}

func NewPlatformFactory() *platformFactory {
	return &platformFactory{}
}

func (f platformFactory) Name() string {
	return "fabric"
}

func (f platformFactory) New(registry api.Context, t api.Topology, builder api.Builder) api.Platform {
	return NewPlatform(registry, t, builder)
}

type Platform struct {
	Network *network.Network
}

func NewPlatform(context api.Context, t api.Topology, components BuilderClient) *Platform {

	// create a new network name
	networkID := common.UniqueName()

	n := network.New(
		context,
		t.(*topology.Topology),
		components,
		[]network.ChaincodeProcessor{},
		networkID,
	)
	p := &Platform{
		Network: n,
	}

	n.AddExtension(fpc.NewExtension(n))

	return p
}

func (p *Platform) Name() string {
	return p.Topology().TopologyName
}

func (p *Platform) Type() string {
	return p.Topology().TopologyType
}

func (p *Platform) GenerateConfigTree() {
	p.Network.GenerateConfigTree()
}

func (p *Platform) GenerateArtifacts() {
	p.Network.GenerateArtifacts()
}

func (p *Platform) Load() {
	p.Network.Load()
}

func (p *Platform) Members() []grouper.Member {
	return p.Network.Members()
}

func (p *Platform) PostRun(load bool) {
	// set up our docker environment for chaincode containers
	err := p.setupDocker()
	Expect(err).NotTo(HaveOccurred())
	p.Network.PostRun(load)
	if load {
		return
	}
	for _, chaincode := range p.Network.Topology().Chaincodes {
		for _, invocation := range chaincode.PostRunInvocations {
			logger.Infof("Post run invocation [%s:%s][%v][%v]",
				chaincode.Chaincode.Name, invocation.FunctionName,
				invocation.ExpectedResult, invocation.Args,
			)
			res := p.InvokeChaincode(
				chaincode,
				invocation.FunctionName,
				invocation.Args...,
			)
			if invocation.ExpectedResult != nil {
				Expect(res).To(BeEquivalentTo(invocation.ExpectedResult))
			}
		}
	}

}

func (p *Platform) Cleanup() {
	p.Network.Cleanup()

	// cleanup docker environment
	err := p.cleanupDocker()
	Expect(err).NotTo(HaveOccurred())
}

func (p *Platform) DeployChaincode(chaincode *topology.ChannelChaincode) {
	p.Network.DeployChaincode(chaincode)
}

func (p *Platform) DeleteVault(id string) {
	fscPeer := p.Network.FSCPeerByName(id)
	Expect(fscPeer).ToNot(BeNil())
	p.Network.FSCNodeVaultDir(fscPeer)
	Expect(os.RemoveAll(p.Network.FSCNodeVaultDir(fscPeer))).ToNot(HaveOccurred())
}

// UpdateChaincode deploys the new version of the chaincode passed by chaincodeId
func (p *Platform) UpdateChaincode(chaincodeId string, version string, path string, packageFile string) {
	p.Network.UpdateChaincode(chaincodeId, version, path, packageFile)
}

func (p *Platform) DefaultIdemixOrgMSPDir() string {
	return p.Network.DefaultIdemixOrgMSPDir()
}

func (p *Platform) Topology() *topology.Topology {
	return p.Network.Topology()
}

func (p *Platform) PeerChaincodeAddress(peerName string) string {
	return p.Network.PeerAddress(p.Network.PeerByName(peerName), network.ChaincodePort)
}

func (p *Platform) OrgMSPID(orgName string) string {
	return p.Network.Organization(orgName).MSPID
}

func (p *Platform) PeerOrgs() []*fabric.Org {
	var orgs []*fabric.Org
	for _, org := range p.Network.PeerOrgs() {
		orgs = append(orgs, &fabric.Org{
			Name:                  org.Name,
			MSPID:                 org.MSPID,
			CACertsBundlePath:     p.Network.CACertsBundlePath(),
			PeerCACertificatePath: p.Network.OrgPeerCACertificatePath(org),
		})
	}
	return orgs
}

func (p *Platform) PeersByOrg(fabricHost string, orgName string, includeAll bool) []*fabric.Peer {
	if len(fabricHost) == 0 {
		if runtime.GOOS == "darwin" {
			fabricHost = "host.docker.internal"
		} else {
			fabricHost = "fabric"
		}
	}

	var peers []*fabric.Peer
	org := p.Network.Organization(orgName)
	for _, peer := range p.Network.PeersInOrg(orgName) {
		if peer.Type != topology.FabricPeer && !includeAll {
			continue
		}
		caCertPath := filepath.Join(p.Network.PeerLocalTLSDir(peer), "ca.crt")

		if peer.Type != topology.FabricPeer {
			peers = append(peers, &fabric.Peer{
				Name:       peer.Name,
				FullName:   fmt.Sprintf("%s.%s", peer.Name, org.Domain),
				TLSCACerts: []string{caCertPath},
				Cert:       p.Network.PeerCert(peer),
			})
		} else {
			_, port, err := net.SplitHostPort(p.Network.PeerAddress(peer, network.OperationsPort))
			Expect(err).NotTo(HaveOccurred())

			peers = append(peers, &fabric.Peer{
				Name:             peer.Name,
				FullName:         fmt.Sprintf("%s.%s", peer.Name, org.Domain),
				ListeningAddress: p.Network.PeerAddress(peer, network.ListenPort),
				TLSCACerts:       []string{caCertPath},
				OperationAddress: net.JoinHostPort(fabricHost, port),
				Cert:             p.Network.PeerCert(peer),
			})
		}
	}
	return peers
}

func (p *Platform) UserByOrg(orgName string, user string) *fabric.User {
	peer := p.Network.PeersInOrg(orgName)[0]

	return &fabric.User{
		Name: user + "@" + p.Network.Organization(orgName).Domain,
		Cert: p.Network.PeerUserCert(peer, user),
		Key:  p.Network.PeerUserKey(peer, user),
	}
}

func (p *Platform) UsersByOrg(orgName string) []*fabric.User {
	org := p.Network.Organization(orgName)
	var users []*fabric.User
	for _, spec := range org.UserSpecs {
		peer := p.Network.PeersInOrg(orgName)[0]
		users = append(users, &fabric.User{
			Name: spec.Name + "@" + p.Network.Organization(orgName).Domain,
			Cert: p.Network.PeerUserCert(peer, spec.Name),
			Key:  p.Network.PeerUserKey(peer, spec.Name),
		})
	}
	return users
}

func (p *Platform) PeersByID(id string) *Peer {
	peer := p.Network.PeerByName(id)
	if peer == nil {
		return nil
	}
	caCertPath := filepath.Join(p.Network.PeerLocalTLSDir(peer), "ca.crt")

	org := p.Network.Organization(peer.Organization)
	result := &Peer{
		Name:       peer.Name,
		FullName:   fmt.Sprintf("%s.%s", peer.Name, org.Domain),
		TLSCACerts: []string{caCertPath},
		Cert:       p.Network.PeerCert(peer),
	}

	if peer.Type == topology.FabricPeer {
		result.ListeningAddress = p.Network.PeerAddress(peer, network.ListenPort)
	}

	result.Identities = append(result.Identities, &Identity{
		ID:    id,
		Type:  "bccsp",
		Path:  p.Network.PeerLocalMSPDir(peer),
		MSPID: org.MSPID,
	})

	for _, identity := range peer.Identities {
		result.Identities = append(result.Identities, &Identity{
			ID:    identity.ID,
			Type:  identity.MSPType,
			Path:  p.Network.PeerLocalExtraIdentityDir(peer, identity.ID),
			MSPID: org.MSPID,
		})
	}

	return result
}

func (p *Platform) Orderers() []*fabric.Orderer {
	fabricHost := "fabric"
	if runtime.GOOS == "darwin" {
		fabricHost = "host.docker.internal"
	}

	var orderers []*fabric.Orderer
	for _, orderer := range p.Network.Orderers {
		caCertPath := filepath.Join(p.Network.OrdererLocalTLSDir(orderer), "ca.crt")
		org := p.Network.Organization(orderer.Organization)

		_, port, err := net.SplitHostPort(p.Network.OrdererAddress(orderer, network.OperationsPort))
		Expect(err).NotTo(HaveOccurred())

		orderers = append(orderers, &fabric.Orderer{
			Name:             orderer.Name,
			FullName:         fmt.Sprintf("%s.%s", orderer.Name, org.Domain),
			ListeningAddress: p.Network.OrdererAddress(orderer, network.ListenPort),
			OperationAddress: net.JoinHostPort(fabricHost, port),
			TLSCACerts:       []string{caCertPath},
		})
	}
	return orderers
}

func (p *Platform) Channels() []*fabric.Channel {
	var channels []*fabric.Channel
	for _, ch := range p.Network.Channels {
		var chaincodes []*fabric.Chaincode
		for _, chaincode := range p.Network.Topology().Chaincodes {
			if chaincode.Channel == ch.Name {
				peers := p.Network.PeersByName(chaincode.Peers)
				var orgs []string
				var orgMSPIDs []string
				for _, peer := range peers {
					found := false
					for _, org := range orgs {
						if org == peer.Organization {
							found = true
							break
						}
					}
					if !found {
						orgs = append(orgs, peer.Organization)
					}
				}
				for _, org := range orgs {
					orgMSPIDs = append(orgMSPIDs, p.OrgMSPID(org))
				}
				chaincodes = append(chaincodes, &fabric.Chaincode{
					Name:      chaincode.Chaincode.Name,
					OrgMSPIDs: orgMSPIDs,
				})
			}
		}
		channels = append(channels, &fabric.Channel{
			Name:       ch.Name,
			Chaincodes: chaincodes,
		})
	}
	return channels
}

func (p *Platform) InvokeChaincode(cc *topology.ChannelChaincode, method string, args ...[]byte) []byte {
	if cc.Private {
		c := contract.GetContract(
			&fpc.ChannelProvider{Network: p.Network, CC: cc},
			cc.Chaincode.Name,
		)
		output, err := c.SubmitTransaction(method, fpc.ArgsToStrings(args)...)
		Expect(err).NotTo(HaveOccurred())
		return output
	}

	orderer := p.Network.Orderer("orderer")
	org := p.PeerOrgs()[0]
	peer := p.Network.Peer(org.Name, p.PeersByOrg("", org.Name, false)[0].Name)
	s := &struct {
		Args []string `json:"Args,omitempty"`
	}{}
	s.Args = append(s.Args, method)
	for _, arg := range args {
		s.Args = append(s.Args, string(arg))
	}
	ctor, err := json.Marshal(s)
	Expect(err).NotTo(HaveOccurred())

	sess, err := p.Network.PeerUserSession(peer, "User1", commands.ChaincodeInvoke{
		ChannelID: cc.Channel,
		Orderer:   p.Network.OrdererAddress(orderer, network.ListenPort),
		Name:      cc.Chaincode.Name,
		Ctor:      string(ctor),
	})
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, p.Network.EventuallyTimeout).Should(gexec.Exit(0))
	Expect(sess.Err).To(gbytes.Say("Chaincode invoke successful. result: status:200"))

	return sess.Buffer().Contents()
}

// ConnectionProfile returns Fabric connection profile
func (p *Platform) ConnectionProfile(name string, ca bool) *network.ConnectionProfile {
	fabricHost := "fabric"
	if runtime.GOOS == "darwin" {
		fabricHost = "host.docker.internal"
	}
	cp := &network.ConnectionProfile{
		Name:                   name,
		Version:                "1.0.0",
		Organizations:          map[string]network.Organization{},
		Peers:                  map[string]network.Peer{},
		CertificateAuthorities: map[string]network.CertificationAuthority{},
	}

	fabricNetwork := p

	// organizations
	orgs := fabricNetwork.PeerOrgs()
	var peerFullNames []string
	for _, org := range orgs {
		peers := fabricNetwork.PeersByOrg("", org.Name, false)
		var names []string
		for _, peer := range peers {
			names = append(names, peer.FullName)
		}

		if ca {
			cp.Organizations[org.Name] = network.Organization{
				MSPID: org.MSPID,
				Peers: names,
				CertificateAuthorities: []string{
					"ca." + name,
				},
			}
		} else {
			signCert, err := ioutil.ReadFile(p.Network.PeerUserCert(p.Network.PeerByName(peers[0].Name), "Admin"))
			Expect(err).NotTo(HaveOccurred())

			adminPrivateKey, err := ioutil.ReadFile(p.Network.PeerUserKey(p.Network.PeerByName(peers[0].Name), "Admin"))
			Expect(err).NotTo(HaveOccurred())

			cp.Organizations[org.Name] = network.Organization{
				MSPID: org.MSPID,
				Peers: names,
				SignedCert: map[string]interface{}{
					"pem": string(signCert),
				},
				AdminPrivateKey: map[string]interface{}{
					"pem": string(adminPrivateKey),
				},
			}
		}

		cp.CertificateAuthorities["ca."+name] = network.CertificationAuthority{
			Url:        "https://127.0.0.1:7054",
			CaName:     "ca." + name,
			TLSCACerts: nil,
			HttpOptions: network.HttpOptions{
				Verify: true,
			},
		}
		for _, peer := range peers {
			_, port, err := net.SplitHostPort(peer.ListeningAddress)
			Expect(err).NotTo(HaveOccurred())

			bb := bytes.Buffer{}

			for _, cert := range peer.TLSCACerts {
				raw, err := ioutil.ReadFile(cert)
				Expect(err).NotTo(HaveOccurred())
				bb.WriteString(string(raw))
			}

			gRPCopts := make(map[string]interface{})
			gRPCopts["request-timeout"] = 120001

			cp.Peers[peer.FullName] = network.Peer{
				URL: "grpcs://" + net.JoinHostPort(fabricHost, port),
				TLSCACerts: map[string]interface{}{
					"pem": bb.String(),
				},
				GrpcOptions: gRPCopts,
			}

			peerFullNames = append(peerFullNames, peer.FullName)
		}

	}

	// orderers
	cp.Orderers = map[string]network.Orderer{}
	var ordererFullNames []string
	for _, orderer := range fabricNetwork.Orderers() {
		_, port, err := net.SplitHostPort(orderer.ListeningAddress)
		Expect(err).NotTo(HaveOccurred())

		bb := bytes.Buffer{}

		for _, cert := range orderer.TLSCACerts {
			raw, err := ioutil.ReadFile(cert)
			Expect(err).NotTo(HaveOccurred())
			bb.WriteString(string(raw))
		}

		gRPCopts := make(map[string]interface{})
		gRPCopts["request-timeout"] = 120001

		cp.Orderers[orderer.FullName] = network.Orderer{
			URL: "grpcs://" + net.JoinHostPort(fabricHost, port),
			TLSCACerts: map[string]interface{}{
				"pem": bb.String(),
			},
			GrpcOptions: gRPCopts,
		}
		ordererFullNames = append(ordererFullNames, orderer.FullName)
	}

	// channels
	cp.Channels = map[string]network.Channel{}
	channelPeers := map[string]network.ChannelPeer{}
	for _, name := range peerFullNames {
		channelPeers[name] = network.ChannelPeer{
			EndorsingPeer:  true,
			ChaincodeQuery: true,
			LedgerQuery:    true,
			EventSource:    true,
			Discover:       true,
		}
	}
	for _, channel := range fabricNetwork.Channels() {
		cp.Channels[channel.Name] = network.Channel{
			Orderers: ordererFullNames,
			Peers:    channelPeers,
		}
	}

	return cp
}
