/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

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

const ServerImage = "orionbcdb/orion-server:latest"

var RequiredImages = []string{ServerImage}

func (p *Platform) StartOrionServer() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	net, err := d.Client.NetworkInfo(p.NetworkID)
	if err != nil {
		panic(err)
	}

	strNodePort := strconv.Itoa(int(p.nodePort))
	strPeerPort := strconv.Itoa(int(p.peerPort))

	containerName := fmt.Sprintf("%s.orion.%s", p.NetworkID, p.Topology.TopologyName)
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: containerName,
		Image:    ServerImage,
		Tty:      false,
		ExposedPorts: nat.PortSet{
			nat.Port(strNodePort + "/tcp"): struct{}{},
			nat.Port(strPeerPort + "/tcp"): struct{}{},
		},
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type: mount.TypeBind,
				// Absolute path to
				Source: p.ledgerDir(),
				Target: "/etc/orion-server/ledger",
			},
			{
				Type: mount.TypeBind,
				// Absolute path to
				Source: p.cryptoDir(),
				Target: "/etc/orion-server/crypto",
			},
			{
				Type: mount.TypeBind,
				// Absolute path to
				Source: p.configDir(),
				Target: "/etc/orion-server/config",
			},
		},
		PortBindings: nat.PortMap{
			nat.Port(strNodePort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: strNodePort,
				},
			},
			nat.Port(strPeerPort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: strPeerPort,
				},
			},
		},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			p.NetworkID: {
				NetworkID: net.ID,
			},
		},
	}, nil, containerName)
	if err != nil {
		panic(err)
	}

	gomega.Expect(cli.NetworkConnect(context.Background(), p.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: p.NetworkID,
	})).ToNot(gomega.HaveOccurred())

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		panic(err)
	}

	dockerLogger := logging.MustGetLogger("orion.container." + containerName)
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

	logger.Debugf("Wait orion to start...")
	time.Sleep(60 * time.Second)
}
