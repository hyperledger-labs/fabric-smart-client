/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"encoding/json"
	"fmt"
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/helpers"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

const CCEnvDefaultImage = "hyperledger/fabric-ccenv:latest"

var (
	RequiredImages = []string{
		CCEnvDefaultImage,
		runner.CouchDBDefaultImage,
		runner.KafkaDefaultImage,
		runner.ZooKeeperDefaultImage,
	}
	once         sync.Once
	dockerClient *docker.Client
)

type Peer struct {
	Name             string
	FullName         string
	ListeningAddress string
	TLSCACerts       []string
}

type Org struct {
	Name             string
	MSPID            string
	CACertsBundlePat string
}

type User struct {
	Name string
	Cert string
	Key  string
}

type Chaincode struct {
	Name string
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

type platform struct {
	Network *network.Network
}

func NewPlatform(context api.Context, t api.Topology, components BuilderClient) *platform {
	helpers.AssertImagesExist(RequiredImages...)

	var err error
	dockerClient, err = docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())
	networkID := common.UniqueName()
	_, err = dockerClient.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:   networkID,
			Driver: "bridge",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	return &platform{
		Network: network.New(
			context,
			t.(*topology.Topology),
			dockerClient,
			components,
			[]network.ChaincodeProcessor{},
			networkID,
		),
	}
}

func (p *platform) Name() string {
	return p.Topology().TopologyName
}

func (p *platform) Type() string {
	return p.Topology().TopologyType
}

func (p *platform) GenerateConfigTree() {
	p.Network.GenerateConfigTree()
}

func (p *platform) GenerateArtifacts() {
	p.Network.GenerateArtifacts()
}

func (p *platform) Load() {
	p.Network.Load()
}

func (p *platform) Members() []grouper.Member {
	return p.Network.Members()
}

func (p *platform) PostRun() {
	p.Network.PostRun()
}

func (p *platform) Cleanup() {
	p.Network.Cleanup()
}

func (p *platform) DeployChaincode(chaincode *topology.ChannelChaincode) {
	p.Network.DeployChaincode(chaincode)
}

func (p *platform) DefaultIdemixOrgMSPDir() string {
	return p.Network.DefaultIdemixOrgMSPDir()
}

func (p *platform) Topology() *topology.Topology {
	return p.Network.Topology()
}

func (p *platform) PeerChaincodeAddress(peerName string) string {
	return p.Network.PeerAddress(p.Network.PeerByName(peerName), network.ChaincodePort)
}

func (p *platform) OrgMSPID(orgName string) string {
	return p.Network.Organization(orgName).MSPID
}

func (p *platform) PeerOrgs() []*Org {
	var orgs []*Org
	for _, org := range p.Network.PeerOrgs() {
		orgs = append(orgs, &Org{
			Name:             org.Name,
			MSPID:            org.MSPID,
			CACertsBundlePat: p.Network.CACertsBundlePath(),
		})
	}
	return orgs
}

func (p *platform) PeersByOrg(orgName string) []*Peer {
	var peers []*Peer
	org := p.Network.Organization(orgName)
	for _, peer := range p.Network.PeersInOrg(orgName) {
		if peer.Type != topology.FabricPeer {
			continue
		}
		peers = append(peers, &Peer{
			Name:             peer.Name,
			FullName:         fmt.Sprintf("%s.%s", peer.Name, org.Domain),
			ListeningAddress: p.Network.PeerAddress(peer, network.ListenPort),
			TLSCACerts:       p.Network.ListTLSCACertificates(),
		})

	}
	return peers
}

func (p *platform) UserByOrg(orgName string, user string) *User {
	peer := p.Network.PeersInOrg(orgName)[0]

	return &User{
		Name: user + "@" + p.Network.Organization(orgName).Domain,
		Cert: p.Network.PeerUserCert(peer, user),
		Key:  p.Network.PeerUserKey(peer, user),
	}
}

func (p *platform) UsersByOrg(orgName string) []*User {
	org := p.Network.Organization(orgName)
	var users []*User
	for _, name := range org.UserNames {
		peer := p.Network.PeersInOrg(orgName)[0]
		users = append(users, &User{
			Name: name + "@" + p.Network.Organization(orgName).Domain,
			Cert: p.Network.PeerUserCert(peer, name),
			Key:  p.Network.PeerUserKey(peer, name),
		})
	}
	return users
}

func (p *platform) Channels() []*Channel {
	var channels []*Channel
	for _, ch := range p.Network.Channels {
		var chaincodes []*Chaincode
		for _, chaincode := range p.Network.Topology().Chaincodes {
			if chaincode.Channel == ch.Name {
				chaincodes = append(chaincodes, &Chaincode{
					Name: chaincode.Chaincode.Name,
				})
			}
		}
		channels = append(channels, &Channel{
			Name:       ch.Name,
			Chaincodes: chaincodes,
		})
	}
	return channels
}

func (p *platform) InvokeChaincode(cc *topology.ChannelChaincode, method string, args ...[]byte) {
	orderer := p.Network.Orderer("orderer")
	org := p.PeerOrgs()[0]
	peer := p.Network.Peer(org.Name, p.PeersByOrg(org.Name)[0].Name)
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
}
