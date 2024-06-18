/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package optl

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	. "github.com/onsi/gomega"
)

const (
	JaegerTracing       = "jaegertracing/all-in-one:latest"
	Collector           = "otel/opentelemetry-collector:latest"
	JaegerQueryPort     = 16685
	JaegerCollectorPort = 4317
	JaegerUIPort        = 16686
	JaegerAdminPort     = 14269
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

	//time.Sleep(10 * time.Second)
	//
	//logger.Infof("Run otel-collector...")
	//n.startOPTLCollector()
	//logger.Infof("Run otel-collector...done!")
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
			Env:          []string{"COLLECTOR_OTLP_ENABLED=true"},
			Hostname:     "jaegertracing.mynetwork.com",
			Image:        JaegerTracing,
			Tty:          false,
			ExposedPorts: docker.PortSet(JaegerCollectorPort, JaegerQueryPort, JaegerUIPort, JaegerAdminPort),
		},
		&container.HostConfig{
			PortBindings: docker.PortBindings([]int{JaegerCollectorPort, JaegerQueryPort, JaegerUIPort, JaegerAdminPort}...),
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

	Expect(cli.ContainerStart(ctx, resp.ID, container.StartOptions{})).ToNot(HaveOccurred())

	Expect(docker.StartLogs(cli, resp.ID, "monitoring.optl.jaegertracing.container")).ToNot(HaveOccurred())
}

//func (n *Extension) startOPTLCollector() {
//	ctx := context.Background()
//	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
//	Expect(err).ToNot(HaveOccurred())
//
//	d, err := docker.GetInstance()
//	Expect(err).NotTo(HaveOccurred())
//
//	net, err := d.Client.NetworkInfo(n.platform.NetworkID())
//	Expect(err).ToNot(HaveOccurred())
//
//	containerName := n.platform.NetworkID() + "-optl.mynetwork.com"
//
//	localIP, err := d.LocalIP(n.platform.NetworkID())
//	Expect(err).ToNot(HaveOccurred())
//
//	logger.Infof("mount at [%s]", n.configFileDir())
//
//	port := strconv.Itoa(n.platform.OPTLPort())
//	resp, err := cli.ContainerCreate(ctx,
//		&container.Config{
//			Hostname: "optl.mynetwork.com",
//			Image:    Collector,
//			Tty:      false,
//			ExposedPorts: nat.PortSet{
//				nat.Port(port + "/tcp"): struct{}{},
//			},
//			Cmd: []string{"--config=/etc/optl-collector-config.yaml"},
//		},
//		&container.HostConfig{
//			ExtraHosts: []string{fmt.Sprintf("fabric:%s", localIP)},
//			Links:      []string{n.platform.NetworkID() + "-jaegertracing.mynetwork.com"},
//			Mounts: []mount.Mount{
//				{
//					Type:   mount.TypeBind,
//					Source: n.configFilePath(),
//					Target: "/etc/optl-collector-config.yaml",
//				},
//			},
//			PortBindings: docker.PortBindings([]int{13133, 4317, 4319, 55679}...),
//		},
//		&network.NetworkingConfig{
//			EndpointsConfig: map[string]*network.EndpointSettings{
//				n.platform.NetworkID(): {
//					NetworkID: net.ID,
//				},
//			},
//		}, nil, containerName,
//	)
//	Expect(err).ToNot(HaveOccurred())
//
//	err = cli.NetworkConnect(context.Background(), n.platform.NetworkID(), resp.ID, &network.EndpointSettings{
//		NetworkID: n.platform.NetworkID(),
//	})
//	Expect(err).ToNot(HaveOccurred())
//
//	Expect(cli.ContainerStart(ctx, resp.ID, container.StartOptions{})).ToNot(HaveOccurred())
//	time.Sleep(3 * time.Second)
//
//	err = docker.StartLogs(cli, resp.ID, "monitoring.optl.container")
//	Expect(err).ToNot(HaveOccurred())
//}
