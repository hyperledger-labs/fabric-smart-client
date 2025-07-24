/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package monitoring

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/onsi/gomega"
)

const (
	PrometheusImg = "prom/prometheus:latest"
	GrafanaImg    = "grafana/grafana:latest"
)

var RequiredImages = []string{
	PrometheusImg,
	GrafanaImg,
}

func (n *Extension) startContainer() {
	// getting our docker helper, check required images exists and launch a docker network
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = d.CheckImagesExist(RequiredImages...)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	logger.Infof("Run Prometheus...")
	n.startPrometheus()
	logger.Infof("Run Prometheus..done!")

	time.Sleep(30 * time.Second)

	logger.Infof("Run Grafana...")
	n.startGrafana()
	logger.Infof("Run Grafana...done!")
}

func (n *Extension) startPrometheus() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// getting our docker helper, check required images exists and launch a docker network
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	net, err := d.Client.NetworkInfo(n.platform.NetworkID())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	containerName := n.platform.NetworkID() + "-prometheus"

	localIP, err := d.LocalIP(n.platform.NetworkID())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	port := strconv.Itoa(n.platform.PrometheusPort())
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: "prometheus",
		Image:    "prom/prometheus:latest",
		Tty:      true,
		ExposedPorts: nat.PortSet{
			nat.Port(port + "/tcp"): struct{}{},
		},
		Cmd: []string{
			"--config.file=/etc/prometheus/prometheus.yml",
			"--storage.tsdb.path=/prometheus",
			"--enable-feature=native-histograms",
		},
	}, &container.HostConfig{
		ExtraHosts:    []string{fmt.Sprintf("fabric:%s", localIP)},
		RestartPolicy: container.RestartPolicy{Name: "always"},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: n.prometheusConfigFilePath(),
				Target: "/etc/prometheus/prometheus.yml",
			},
			{
				Type:   mount.TypeBind,
				Source: n.fscCryptoDir(),
				Target: "/etc/prometheus/fsc/crypto",
			},
		},
		PortBindings: nat.PortMap{
			nat.Port(port + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: port,
				},
			},
		},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			n.platform.NetworkID(): {
				NetworkID: net.ID,
			},
		},
	}, nil, containerName,
	)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.platform.NetworkID(), resp.ID, &network.EndpointSettings{
		NetworkID: n.platform.NetworkID(),
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	gomega.Expect(cli.ContainerStart(ctx, resp.ID, container.StartOptions{})).ToNot(gomega.HaveOccurred())

	dockerLogger := logging.MustGetLogger()
	go func() {
		reader, err := cli.ContainerLogs(context.Background(), resp.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer utils.IgnoreErrorFunc(reader.Close)

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			dockerLogger.Debugf("%s", scanner.Text())
		}
	}()

	logger.Infof("Prometheus running on http://localhost:%s", port)
}

func (n *Extension) startGrafana() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	net, err := d.Client.NetworkInfo(n.platform.NetworkID())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	containerName := n.platform.NetworkID() + "-grafana"

	port := strconv.Itoa(n.platform.GrafanaPort())
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: "grafana",
		Image:    GrafanaImg,
		Tty:      false,
		Env: []string{
			"GF_AUTH_PROXY_ENABLED=true",
			"GF_PATHS_PROVISIONING=/var/lib/grafana/provisioning/",
		},
		ExposedPorts: nat.PortSet{
			nat.Port(port + "/tcp"): struct{}{},
		},
	}, &container.HostConfig{
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
					HostIP:   "0.0.0.0",
					HostPort: port,
				},
			},
		},
	},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				n.platform.NetworkID(): {
					NetworkID: net.ID,
				},
			},
		}, nil, containerName,
	)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.platform.NetworkID(), resp.ID, &network.EndpointSettings{
		NetworkID: n.platform.NetworkID(),
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	gomega.Expect(cli.ContainerStart(ctx, resp.ID, container.StartOptions{})).ToNot(gomega.HaveOccurred())
	time.Sleep(3 * time.Second)

	dockerLogger := logging.MustGetLogger()
	go func() {
		reader, err := cli.ContainerLogs(context.Background(), resp.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		defer utils.IgnoreErrorFunc(reader.Close)

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			dockerLogger.Debugf("%s", scanner.Text())
		}
	}()
	logger.Infof("Grafana running on http://localhost:%s with username: 'admin', password: 'admin'", port)
}
