/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"sync"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger/fabric/integration/runner"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
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
	return "default"
}

func (p *platform) Type() string {
	return "fabric"
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
