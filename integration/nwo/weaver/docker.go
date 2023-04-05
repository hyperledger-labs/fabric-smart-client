/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"bufio"
	"context"
	"fmt"

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

func (p *Platform) RunRelayServer(name string, serverConfigPath, port string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	net, err := d.Client.NetworkInfo(p.NetworkID)
	Expect(err).ToNot(HaveOccurred())

	hostname := "relay-" + name

	driverName := "driver-" + name

	var links []string

	if name == "beta" {
		links = []string{"relay-alpha:relay-alpha"}
	}

	links = append(links, fmt.Sprintf("%s:%s", driverName, driverName))

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: hostname,
		Image:    RelayServerImage,
		Tty:      false,
		Env: []string{
			"DEBUG=true",
			"RELAY_CONFIG=/opt/relay/config/server.toml",
			"RELAY_TLS=false",
			"DRIVER_TLS=false",
		},
		ExposedPorts: nat.PortSet{
			nat.Port(port + "/tcp"): struct{}{},
		},
	}, &container.HostConfig{
		Links: links,
		Mounts: []mount.Mount{
			{
				Type: mount.TypeBind,
				// Absolute path to
				Source: serverConfigPath,
				Target: "/opt/relay/config/server.toml",
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
			p.NetworkID: {
				NetworkID: net.ID,
			},
		},
	}, nil, hostname)
	Expect(err).ToNot(HaveOccurred())

	cli.NetworkConnect(context.Background(), p.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: p.NetworkID,
	})

	err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	Expect(err).ToNot(HaveOccurred())

	dockerLogger := flogging.MustGetLogger("weaver.container." + hostname)
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

func (p *Platform) RunRelayFabricDriver(
	networkName,
	relayHost, relayPort,
	driverHost, driverPort,
	interopChaincode,
	ccpPath, configPath, walletPath string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	hostname := "driver-" + networkName

	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	net, err := d.Client.NetworkInfo(p.NetworkID)
	Expect(err).ToNot(HaveOccurred())

	localIP, err := d.LocalIP(p.NetworkID)
	Expect(err).ToNot(HaveOccurred())

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: hostname,
		Image:    FabricDriverImage,
		Tty:      false,
		Env: []string{
			"NETWORK_NAME=" + networkName,
			"RELAY_ENDPOINT=" + relayHost + ":" + relayPort,
			"DRIVER_ENDPOINT=" + driverHost + ":" + driverPort,
			"DRIVER_CONFIG=/config.json",
			"CONNECTION_PROFILE=/ccp.json",
			"INTEROP_CHAINCODE=" + interopChaincode,
			"local=false",
			//"GRPC_TRACE=all",
			//"GRPC_VERBOSITY=DEBUG",
			//"GRPC_NODE_VERBOSITY=DEBUG",
			//"GRPC_NODE_TRACE=connectivity_state,server,server_call,subchannel",
			//"NODE_OPTIONS=--tls-max-v1.2",
			// "HFC_LOGGING={\"debug\":\"console\",\"info\":\"console\"}",
		},
		Cmd: []string{
			"npm", "run", "dev", "--verbose=true",
			// "/bin/sh", "run.sh",
		},
		ExposedPorts: nat.PortSet{
			nat.Port(driverPort + "/tcp"): struct{}{},
			nat.Port(relayPort + "/tcp"):  struct{}{},
		},
	}, &container.HostConfig{
		ExtraHosts: []string{fmt.Sprintf("fabric:%s", localIP)},
		// Absolute path to
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: ccpPath,
				Target: "/ccp.json",
			},
			{
				Type:   mount.TypeBind,
				Source: walletPath,
				Target: "/fabric-driver/wallet-" + networkName,
			},
			{
				Type:   mount.TypeBind,
				Source: configPath,
				Target: "/config.json",
			},
		},
		PortBindings: nat.PortMap{
			nat.Port(driverPort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: driverPort,
				},
			},
		},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			p.NetworkID: {
				NetworkID: net.ID,
			},
		},
	}, nil, hostname)
	Expect(err).ToNot(HaveOccurred())

	cli.NetworkConnect(context.Background(), p.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: p.NetworkID,
	})

	err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	Expect(err).ToNot(HaveOccurred())

	dockerLogger := flogging.MustGetLogger("weaver.fabric.driver.container." + hostname)
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
