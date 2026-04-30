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
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

const PrometheusPort = 9090

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

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		logger.Infof("Run Prometheus...")
		n.startPrometheus()
		logger.Infof("Run Prometheus..done!")
	}()

	go func() {
		defer wg.Done()
		logger.Infof("Run Grafana...")
		n.startGrafana()
		logger.Infof("Run Grafana...done!")
	}()

	wg.Wait()
}

func (n *Extension) startPrometheus() {
	ctx := context.TODO()
	cli, err := dcli.New(dcli.FromEnv)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	// getting our docker helper, check required images exists and launch a docker network
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	net, err := d.NetworkInfo(n.platform.NetworkID())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	containerName := n.platform.NetworkID() + "-prometheus"

	localIP, err := d.LocalIP(n.platform.NetworkID())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	port := strconv.Itoa(n.platform.PrometheusPort())
	resp, err := cli.ContainerCreate(ctx, dcli.ContainerCreateOptions{
		Name: containerName,
		Config: &container.Config{
			Hostname:     "prometheus",
			Image:        "prom/prometheus:latest",
			Tty:          true,
			ExposedPorts: docker.PortSet(n.platform.PrometheusPort()),
			Cmd: []string{
				"--config.file=/etc/prometheus/prometheus.yml",
				"--storage.tsdb.path=/prometheus",
				"--enable-feature=native-histograms",
			},
		},
		HostConfig: &container.HostConfig{
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
			PortBindings: docker.PortBindings(n.platform.PrometheusPort()),
		},
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				n.platform.NetworkID(): {
					NetworkID: net.ID,
				},
			},
		},
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	_, err = cli.NetworkConnect(context.TODO(), n.platform.NetworkID(), dcli.NetworkConnectOptions{
		Container: resp.ID,
		EndpointConfig: &network.EndpointSettings{
			NetworkID: n.platform.NetworkID(),
		},
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	_, err = cli.ContainerStart(ctx, resp.ID, dcli.ContainerStartOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	dockerLogger := logging.MustGetLogger()
	go func() {
		reader, err := cli.ContainerLogs(context.TODO(), resp.ID, dcli.ContainerLogsOptions{
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
	ctx := context.TODO()
	cli, err := dcli.New(dcli.FromEnv)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	net, err := d.NetworkInfo(n.platform.NetworkID())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	containerName := n.platform.NetworkID() + "-grafana"

	port := strconv.Itoa(n.platform.GrafanaPort())
	resp, err := cli.ContainerCreate(ctx, dcli.ContainerCreateOptions{
		Name: containerName,
		Config: &container.Config{
			Hostname: "grafana",
			Image:    GrafanaImg,
			Tty:      false,
			Env: []string{
				"GF_AUTH_PROXY_ENABLED=true",
				"GF_PATHS_PROVISIONING=/var/lib/grafana/provisioning/",
			},
			ExposedPorts: docker.PortSet(n.platform.GrafanaPort()),
		},
		HostConfig: &container.HostConfig{
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
			PortBindings: docker.PortBindings(n.platform.GrafanaPort()),
		},
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				n.platform.NetworkID(): {
					NetworkID: net.ID,
				},
			},
		},
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	_, err = cli.NetworkConnect(context.TODO(), n.platform.NetworkID(), dcli.NetworkConnectOptions{
		Container: resp.ID,
		EndpointConfig: &network.EndpointSettings{
			NetworkID: n.platform.NetworkID(),
		},
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	_, err = cli.ContainerStart(ctx, resp.ID, dcli.ContainerStartOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	time.Sleep(3 * time.Second)

	dockerLogger := logging.MustGetLogger()
	go func() {
		reader, err := cli.ContainerLogs(context.TODO(), resp.ID, dcli.ContainerLogsOptions{
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
