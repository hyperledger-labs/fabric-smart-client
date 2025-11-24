/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otlp

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/onsi/gomega"
)

const (
	JaegerTracing       = "cr.jaegertracing.io/jaegertracing/jaeger:2.12.0"
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
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = d.CheckImagesExist(RequiredImages...)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	logger.Infof("Run jaeger-all-in-one...")
	n.startJaeger()
	logger.Infof("Run jaeger-all-in-one...done!")
}

func (n *Extension) startJaeger() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	net, err := d.Client.NetworkInfo(n.platform.NetworkID())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

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
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.platform.NetworkID(), resp.ID, &network.EndpointSettings{
		NetworkID: n.platform.NetworkID(),
	})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	gomega.Expect(cli.ContainerStart(ctx, resp.ID, container.StartOptions{})).ToNot(gomega.HaveOccurred())

	logger.Infof("Follow the traces on localhost:%d", JaegerUIPort)
	gomega.Expect(docker.StartLogs(cli, resp.ID, "monitoring.otlp.jaegertracing.container")).ToNot(gomega.HaveOccurred())
}
