/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"github.com/hyperledger/fabric/integration/runner"
	"github.com/tedsuo/ifrit/grouper"

	registry2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/registry"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/helpers"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

const CCEnvDefaultImage = "hyperledger/fabric-ccenv:latest"

var RequiredImages = []string{
	CCEnvDefaultImage,
	runner.CouchDBDefaultImage,
	runner.KafkaDefaultImage,
	runner.ZooKeeperDefaultImage,
}

type BuilderClient interface {
	Build(path string) string
}

type platform struct {
	Network *network.Network
}

func NewPlatform(registry *registry2.Registry, components BuilderClient) *platform {
	helpers.AssertImagesExist(RequiredImages...)

	return &platform{
		Network: network.New(
			registry,
			components,
			[]network.ChaincodeProcessor{},
		),
	}
}

func (p *platform) Name() string {
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
