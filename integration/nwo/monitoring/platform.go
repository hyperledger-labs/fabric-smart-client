/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

import (
	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/hle"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/grouper"
	"path/filepath"
	"strings"
	"time"
)

var logger = flogging.MustGetLogger("fsc.integration.monitoring")

const (
	TopologyName = "monitoring"
)

type platformFactory struct{}

func NewPlatformFactory() *platformFactory {
	return &platformFactory{}
}

func (f platformFactory) Name() string {
	return TopologyName
}

func (f platformFactory) New(registry api.Context, t api.Topology, builder api.Builder) api.Platform {
	return New(registry, t.(*Topology))
}

type Extension interface {
	CheckTopology()
	GenerateArtifacts()
	PostRun(load bool)
}

type Platform struct {
	Context      api.Context
	topology     *Topology
	RootDir      string
	Prefix       string
	Extensions   []Extension
	networkID    string
	dockerClient *docker.Client
}

func New(reg api.Context, topology *Topology) *Platform {
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

	p := &Platform{
		Context:      reg,
		RootDir:      reg.RootDir(),
		Prefix:       topology.Name(),
		topology:     topology,
		Extensions:   []Extension{},
		dockerClient: dockerClient,
		networkID:    networkID,
	}
	p.AddExtension(hle.NewExtension(p))
	p.AddExtension(monitoring.NewExtension(p))

	return p
}

func (p *Platform) Name() string {
	return p.topology.Name()
}

func (p *Platform) Type() string {
	return p.topology.Type()
}

func (p *Platform) GenerateConfigTree() {
}

func (p *Platform) GenerateArtifacts() {
	for _, extension := range p.Extensions {
		extension.CheckTopology()
		extension.GenerateArtifacts()
	}
}

func (p *Platform) Load() {
}

func (p *Platform) Members() []grouper.Member {
	return nil
}

func (p *Platform) PostRun(load bool) {
	logger.Infof("Post execution [%s]...", p.Prefix)

	// Extensions
	for _, extension := range p.Extensions {
		extension.PostRun(load)
	}

	// Wait a few second to let Fabric stabilize
	time.Sleep(5 * time.Second)
	logger.Infof("Post execution [%s]...done.", p.Prefix)
}

func (p *Platform) Cleanup() {
	if p.dockerClient == nil {
		return
	}

	nw, err := p.dockerClient.NetworkInfo(p.networkID)
	if _, ok := err.(*docker.NoSuchNetwork); err != nil && ok {
		return
	}
	Expect(err).NotTo(HaveOccurred())

	containers, err := p.dockerClient.ListContainers(docker.ListContainersOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.HasPrefix(name, "/"+p.networkID) {
				logger.Infof("cleanup container [%s]", name)
				err := p.dockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: c.ID, Force: true})
				Expect(err).NotTo(HaveOccurred())
				break
			} else {
				logger.Infof("cleanup container [%s], skipped", name)
			}
		}
	}

	volumes, err := p.dockerClient.ListVolumes(docker.ListVolumesOptions{})
	Expect(err).NotTo(HaveOccurred())
	for _, i := range volumes {
		if strings.HasPrefix(i.Name, p.networkID) {
			logger.Infof("cleanup volume [%s]", i.Name)
			err := p.dockerClient.RemoveVolumeWithOptions(docker.RemoveVolumeOptions{
				Name:  i.Name,
				Force: false,
			})
			Expect(err).NotTo(HaveOccurred())
			break
		}
	}

	images, err := p.dockerClient.ListImages(docker.ListImagesOptions{All: true})
	Expect(err).NotTo(HaveOccurred())
	for _, i := range images {
		for _, tag := range i.RepoTags {
			if strings.HasPrefix(tag, p.networkID) {
				logger.Infof("cleanup image [%s]", tag)
				err := p.dockerClient.RemoveImage(i.ID)
				Expect(err).NotTo(HaveOccurred())
				break
			}
		}
	}

	err = p.dockerClient.RemoveNetwork(nw.ID)
	Expect(err).NotTo(HaveOccurred())
}

func (p *Platform) AddExtension(ex Extension) {
	p.Extensions = append(p.Extensions, ex)
}

func (p *Platform) GetContext() api.Context {
	return p.Context
}

func (p *Platform) DockerClient() *docker.Client {
	return p.dockerClient
}

func (p *Platform) NetworkID() string {
	return p.networkID
}

func (p *Platform) ConfigDir() string {
	return filepath.Join(p.RootDir, "monitoring")
}

func (p *Platform) HyperledgerExplorer() bool {
	return p.topology.HyperledgerExplorer
}

func (p *Platform) HyperledgerExplorerPort() int {
	return p.topology.HyperledgerExplorerPort
}

func (p *Platform) PrometheusGrafana() bool {
	return p.topology.PrometheusGrafana
}

func (p *Platform) PrometheusPort() int {
	return p.topology.PrometheusPort
}

func (p *Platform) GrafanaPort() int {
	return p.topology.GrafanaPort
}
