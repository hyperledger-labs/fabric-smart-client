/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otlp

import (
	"context"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"
	"github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
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
	ctx := context.TODO()
	cli, err := dcli.New(dcli.FromEnv)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	net, err := d.NetworkInfo(n.platform.NetworkID())
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	containerName := n.platform.NetworkID() + "-jaegertracing.mynetwork.com"

	resp, err := cli.ContainerCreate(ctx, dcli.ContainerCreateOptions{
		Name: containerName,
		Config: &container.Config{
			Env:          []string{"COLLECTOR_OTLP_ENABLED=true"},
			Hostname:     "jaegertracing.mynetwork.com",
			Image:        JaegerTracing,
			Tty:          false,
			ExposedPorts: docker.PortSet(JaegerCollectorPort, JaegerQueryPort, JaegerUIPort, JaegerAdminPort),
		},
		HostConfig: &container.HostConfig{
			PortBindings: docker.PortBindings([]int{JaegerCollectorPort, JaegerQueryPort, JaegerUIPort, JaegerAdminPort}...),
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

	logger.Infof("Follow the traces on localhost:%d", JaegerUIPort)
	gomega.Expect(docker.StartLogs(cli, resp.ID, "monitoring.otlp.jaegertracing.container")).ToNot(gomega.HaveOccurred())
}
