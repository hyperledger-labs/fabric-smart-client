/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package optl

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	. "github.com/onsi/gomega"
)

const (
	JaegerTracing = "jaegertracing/all-in-one:latest"
	Collector     = "otel/opentelemetry-collector:latest"
)

var RequiredImages = []string{
	JaegerTracing,
	Collector,
}

func (n *Extension) startContainer() {
	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	err = d.CheckImagesExist(RequiredImages...)
	Expect(err).NotTo(HaveOccurred())

	logger.Infof("Run jaeger-all-in-one...")
	n.startJaeger()
	logger.Infof("Run jaeger-all-in-one...done!")

	time.Sleep(10 * time.Second)

	logger.Infof("Run otel-collector...")
	n.startOPTLCollector()
	logger.Infof("Run otel-collector...done!")
}

func (n *Extension) startJaeger() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	net, err := d.Client.NetworkInfo(n.platform.NetworkID())
	Expect(err).ToNot(HaveOccurred())

	containerName := n.platform.NetworkID() + "-jaegertracing.mynetwork.com"

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Hostname: "jaegertracing.mynetwork.com",
			Image:    JaegerTracing,
			Tty:      false,
		},
		&container.HostConfig{
			PortBindings: nat.PortMap{
				nat.Port("16686" + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: "16686",
					},
				},
				nat.Port("14268" + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: "14268",
					},
				},
				nat.Port("14250" + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: "14250",
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
	Expect(err).ToNot(HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.platform.NetworkID(), resp.ID, &network.EndpointSettings{
		NetworkID: n.platform.NetworkID(),
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})).ToNot(HaveOccurred())

	dockerLogger := flogging.MustGetLogger("monitoring.optl.jaegertracing.container")
	go func() {
		reader, err := cli.ContainerLogs(context.Background(), resp.ID, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		Expect(err).ToNot(HaveOccurred())
		defer reader.Close()

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			dockerLogger.Debugf("%s", scanner.Text())
		}
	}()

}

func (n *Extension) startOPTLCollector() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	net, err := d.Client.NetworkInfo(n.platform.NetworkID())
	Expect(err).ToNot(HaveOccurred())

	containerName := n.platform.NetworkID() + "-optl.mynetwork.com"

	localIP, err := d.LocalIP(n.platform.NetworkID())
	Expect(err).ToNot(HaveOccurred())

	logger.Infof("mount at [%s]", n.configFileDir())

	port := strconv.Itoa(n.platform.OPTLPort())
	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Hostname: "optl.mynetwork.com",
			Image:    Collector,
			Tty:      false,
			ExposedPorts: nat.PortSet{
				nat.Port(port + "/tcp"): struct{}{},
			},
			Cmd: []string{"--config=/etc/optl-collector-config.yaml"},
		},
		&container.HostConfig{
			ExtraHosts: []string{fmt.Sprintf("fabric:%s", localIP)},
			Links:      []string{n.platform.NetworkID() + "-jaegertracing.mynetwork.com"},
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: n.configFilePath(),
					Target: "/etc/optl-collector-config.yaml",
				},
			},
			PortBindings: nat.PortMap{
				nat.Port("13133" + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: "13133",
					},
				},
				nat.Port("4317" + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: "4317",
					},
				},
				nat.Port("4319" + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: "4319",
					},
				},
				nat.Port("55679" + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: "55679",
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
	Expect(err).ToNot(HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.platform.NetworkID(), resp.ID, &network.EndpointSettings{
		NetworkID: n.platform.NetworkID(),
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})).ToNot(HaveOccurred())
	time.Sleep(3 * time.Second)

	dockerLogger := flogging.MustGetLogger("monitoring.optl.container")
	go func() {
		reader, err := cli.ContainerLogs(context.Background(), resp.ID, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		Expect(err).ToNot(HaveOccurred())
		defer reader.Close()

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			dockerLogger.Debugf("%s", scanner.Text())
		}
	}()
}
