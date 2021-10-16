/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"

	api2 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/helpers"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

var RequiredImages = []string{}

type Builder interface {
	Build(path string) string
}

type FabricNetwork interface {
	DeployChaincode(chaincode *topology.ChannelChaincode)
	DefaultIdemixOrgMSPDir() string
	Topology() *topology.Topology
	PeerChaincodeAddress(peerName string) string
	PeerOrgs() []*fabric.Org
	OrgMSPID(orgName string) string
	PeersByOrg(orgName string, includeAll bool) []*fabric.Peer
	UserByOrg(organization string, user string) *fabric.User
	UsersByOrg(organization string) []*fabric.User
	Orderers() []*fabric.Orderer
	Channels() []*fabric.Channel
	InvokeChaincode(cc *topology.ChannelChaincode, method string, args ...[]byte)
}

type platformFactory struct{}

func NewPlatformFactory() *platformFactory {
	return &platformFactory{}
}

func (f platformFactory) Name() string {
	return TopologyName
}

func (f platformFactory) New(registry api2.Context, t api2.Topology, builder api2.Builder) api2.Platform {
	return NewPlatform(registry, t, builder)
}

type Platform struct {
	Context           api2.Context
	Topology          *Topology
	Builder           api2.Builder
	EventuallyTimeout time.Duration

	NetworkID    string
	DockerClient *docker.Client

	colorIndex int
}

func NewPlatform(ctx api2.Context, t api2.Topology, builder api2.Builder) *Platform {
	helpers.AssertImagesExist(RequiredImages...)

	dockerClient, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())
	networkID := common.UniqueName()
	_, err = dockerClient.CreateNetwork(
		docker.CreateNetworkOptions{
			Name:   networkID,
			Driver: "bridge",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	return &Platform{
		Context:           ctx,
		Topology:          t.(*Topology),
		Builder:           builder,
		EventuallyTimeout: 10 * time.Minute,
		NetworkID:         networkID,
		DockerClient:      dockerClient,
	}
}

func (p *Platform) Name() string {
	return TopologyName
}

func (p *Platform) Type() string {
	return TopologyName
}

func (p *Platform) GenerateConfigTree() {
}

func (p *Platform) GenerateArtifacts() {
}

func (p *Platform) Load() {
}

func (p *Platform) Members() []grouper.Member {
	return nil
}

func (p *Platform) PostRun() {
}

func (p *Platform) Cleanup() {
	if p.DockerClient == nil {
		return
	}

	nw, err := p.DockerClient.NetworkInfo(p.NetworkID)
	if _, ok := err.(*docker.NoSuchNetwork); err != nil && ok {
		return
	}
	Expect(err).NotTo(HaveOccurred())

	containers, err := p.DockerClient.ListContainers(docker.ListContainersOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, c := range containers {
		for _, name := range c.Names {
			p.DockerClient.DisconnectNetwork(p.NetworkID, docker.NetworkConnectionOptions{
				Force:     true,
				Container: c.ID,
			})
			if strings.HasPrefix(name, "/orion") {
				err := p.DockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID, Force: true})
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}

	images, err := p.DockerClient.ListImages(docker.ListImagesOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, i := range images {
		for _, tag := range i.RepoTags {
			if strings.HasPrefix(tag, p.NetworkID) {
				err := p.DockerClient.RemoveImage(i.ID)
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}

	err = p.DockerClient.RemoveNetwork(nw.ID)
	Expect(err).NotTo(HaveOccurred())
}
