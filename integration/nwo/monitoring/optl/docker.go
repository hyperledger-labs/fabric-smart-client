/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package optl

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	. "github.com/onsi/gomega"
)

const (
	JaegerTracing       = "jaegertracing/all-in-one:latest"
	JaegerQueryPort     = 16685
	JaegerCollectorPort = 4317
	JaegerUIPort        = 16686
	JaegerAdminPort     = 14269
)

var RequiredImages = []string{
	JaegerTracing,
}

func (n *Extension) startContainer() {
	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	err = d.CheckImagesExist(RequiredImages...)
	Expect(err).NotTo(HaveOccurred())

	logger.Infof("Run jaeger-all-in-one...")
	n.startJaeger()
	logger.Infof("Run jaeger-all-in-one...done!")
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
			Mounts: []mount.Mount{
				// To avoid error: "error reading server preface: EOF"
				{
					Type:   mount.TypeBind,
					Source: n.jaegerHostsPath(),
					Target: "/etc/hosts",
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

	Expect(cli.ContainerStart(ctx, resp.ID, container.StartOptions{})).ToNot(HaveOccurred())

	logger.Infof("Follow the traces on localhost:%d", JaegerUIPort)
	Expect(docker.StartLogs(cli, resp.ID, "monitoring.optl.jaegertracing.container")).ToNot(HaveOccurred())
}
