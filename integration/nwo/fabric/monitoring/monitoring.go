/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/helpers"
	nnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const (
	PrometheusImg = "prom/prometheus:latest"
	GrafanaImg    = "grafana/grafana:latest"
)

var RequiredImages = []string{
	PrometheusImg,
	GrafanaImg,
}

var logger = flogging.MustGetLogger("integration.nwo.fabric.monitoring")

type Platform interface {
	ConnectionProfile(name string, ca bool) *nnetwork.ConnectionProfile
	Orderers() []*fabric.Orderer
	PeerOrgs() []*fabric.Org
	PeersByOrg(orgName string, includeAll bool) []*fabric.Peer
}

type Extension struct {
	network  *nnetwork.Network
	platform Platform
}

func NewExtension(network *nnetwork.Network, cpp Platform) *Extension {
	return &Extension{
		network:  network,
		platform: cpp,
	}
}

func (n *Extension) CheckTopology() {
	if !n.network.Topology().Monitoring {
		return
	}

	helpers.AssertImagesExist(RequiredImages...)

	return
}

func (n *Extension) GenerateArtifacts() {
	// Generate and store prometheus config
	prometheusConfig := Prometheus{
		Global: Global{
			ScrapeInterval:     "15s",
			EvaluationInterval: "15s",
		},
		ScrapeConfigs: []ScrapeConfig{
			{
				JobName: "prometheus",
				StaticConfigs: []StaticConfig{
					{
						Targets: []string{"prometheus:9090"},
					},
				},
			},
		},
	}

	osc := ScrapeConfig{
		JobName: "orderers",
		StaticConfigs: []StaticConfig{
			{
				Targets: []string{},
			},
		},
	}
	for _, orderer := range n.platform.Orderers() {
		osc.StaticConfigs[0].Targets = append(osc.StaticConfigs[0].Targets, orderer.OperationAddress)
	}
	prometheusConfig.ScrapeConfigs = append(prometheusConfig.ScrapeConfigs, osc)
	for _, org := range n.platform.PeerOrgs() {
		sc := ScrapeConfig{
			JobName: "Peers in " + org.Name,
			StaticConfigs: []StaticConfig{
				{
					Targets: []string{},
				},
			},
		}
		for _, peer := range n.platform.PeersByOrg(org.Name, false) {
			sc.StaticConfigs[0].Targets = append(sc.StaticConfigs[0].Targets, peer.OperationAddress)
		}
		prometheusConfig.ScrapeConfigs = append(prometheusConfig.ScrapeConfigs, sc)
	}
	// marshal config
	configYAML, err := yaml.Marshal(prometheusConfig)
	Expect(err).NotTo(HaveOccurred())
	// write config to file
	Expect(os.MkdirAll(n.configFileDir(), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(n.prometheusConfigFilePath(), configYAML, 0o644)).NotTo(HaveOccurred())

	// Generate grafana's provisioning
	for _, dir := range n.grafanaDirPaths() {
		Expect(os.MkdirAll(dir, 0o755)).NotTo(HaveOccurred())
	}
	Expect(ioutil.WriteFile(n.grafanaProvisioningDashboardFilePath(), []byte(DashboardTemplate), 0o644)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(n.grafanaProvisioningDatasourceFilePath(), []byte(DatasourceTemplate), 0o644)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(n.grafanaDashboardFabricBackendFilePath(), []byte(DashboardFabricBackendTemplate), 0o644)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(n.grafanaDashboardFabricBusinessFilePath(), []byte(DashboardFabricBusinessTemplate), 0o644)).NotTo(HaveOccurred())

}

func (n *Extension) PostRun() {
	logger.Infof("Run Prometheus [%s]...", n.network.Topology().Name())
	n.dockerPrometheus()
	logger.Infof("Run Prometheus [%s]...done!", n.network.Topology().Name())
	time.Sleep(30 * time.Second)
	logger.Infof("Run Grafana [%s]...", n.network.Topology().Name())
	n.dockerGrafana()
	logger.Infof("Run Grafana [%s]...done!", n.network.Topology().Name())
}

func (n *Extension) checkTopology() {
}

func (n *Extension) dockerPrometheus() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	net, err := n.network.DockerClient.NetworkInfo(n.network.NetworkID)
	Expect(err).ToNot(HaveOccurred())

	port := "9090"
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: "prometheus",
		Image:    "prom/prometheus:latest",
		Tty:      true,
		ExposedPorts: nat.PortSet{
			nat.Port(port + "/tcp"): struct{}{},
		},
	}, &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "always"},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: n.prometheusConfigFilePath(),
				Target: "/etc/prometheus/prometheus.yml",
			},
		},
		PortBindings: nat.PortMap{
			nat.Port(port + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: port,
				},
			},
		},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			n.network.NetworkID: {
				NetworkID: net.ID,
			},
		},
	}, nil, "prometheus",
	)
	Expect(err).ToNot(HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.network.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: n.network.NetworkID,
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})).ToNot(HaveOccurred())
}

func (n *Extension) dockerGrafana() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	net, err := n.network.DockerClient.NetworkInfo(n.network.NetworkID)
	Expect(err).ToNot(HaveOccurred())

	port := "3000"
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: "grafana",
		Image:    "grafana/grafana:latest",
		Tty:      false,
		Env: []string{
			"GF_AUTH_PROXY_ENABLED=true",
			"GF_PATHS_PROVISIONING=/var/lib/grafana/provisioning/",
		},
		ExposedPorts: nat.PortSet{
			nat.Port(port + "/tcp"): struct{}{},
		},
	}, &container.HostConfig{
		Links: []string{"prometheus"},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: n.grafanaProvisioningDirPath(),
				Target: "/var/lib/grafana/provisioning/",
			},
			{
				Type:   mount.TypeBind,
				Source: n.grafanaDashboardDirPath(),
				Target: "/var/lib/grafana/dashboards/",
			},
		},
		PortBindings: nat.PortMap{
			nat.Port(port + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: port,
				},
			},
		},
	},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				n.network.NetworkID: {
					NetworkID: net.ID,
				},
			},
		}, nil, "grafana",
	)
	Expect(err).ToNot(HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.network.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: n.network.NetworkID,
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})).ToNot(HaveOccurred())
	time.Sleep(3 * time.Second)
}

func (n *Extension) configFileDir() string {
	return filepath.Join(
		n.network.ConfigDir(),
		"monitoring",
	)
}

func (n *Extension) grafanaDirPaths() []string {
	return []string{
		filepath.Join(
			n.configFileDir(),
			"grafana",
			"dashboards",
		),
		filepath.Join(
			n.configFileDir(),
			"grafana",
			"provisioning",
		),
		filepath.Join(
			n.configFileDir(),
			"grafana",
			"provisioning",
			"dashboards",
		),
		filepath.Join(
			n.configFileDir(),
			"grafana",
			"provisioning",
			"datasources",
		),
		filepath.Join(
			n.configFileDir(),
			"grafana",
			"provisioning",
			"notifiers",
		),
	}
}

func (n *Extension) grafanaProvisioningDirPath() string {
	return filepath.Join(
		n.configFileDir(),
		"grafana",
		"provisioning",
	)
}

func (n *Extension) grafanaProvisioningDashboardFilePath() string {
	return filepath.Join(
		n.configFileDir(),
		"grafana",
		"provisioning",
		"dashboards",
		"dashboard.yaml",
	)
}

func (n *Extension) grafanaProvisioningDatasourceFilePath() string {
	return filepath.Join(
		n.configFileDir(),
		"grafana",
		"provisioning",
		"datasources",
		"datasource.yaml",
	)
}

func (n *Extension) grafanaDashboardDirPath() string {
	return filepath.Join(
		n.configFileDir(),
		"grafana",
		"dashboards",
	)
}

func (n *Extension) grafanaDashboardFabricBackendFilePath() string {
	return filepath.Join(
		n.configFileDir(),
		"grafana",
		"dashboards",
		"fabric_backed.json",
	)
}

func (n *Extension) grafanaDashboardFabricBusinessFilePath() string {
	return filepath.Join(
		n.configFileDir(),
		"grafana",
		"dashboards",
		"fabric_business.json",
	)
}

func (n *Extension) prometheusConfigFilePath() string {
	return filepath.Join(n.configFileDir(), "prometheus.yml")
}
