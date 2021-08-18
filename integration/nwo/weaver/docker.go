/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/network"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

func (p *Platform) RunRelayServer(name string, serverConfigPath, port string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	net, err := p.DockerClient.NetworkInfo(p.NetworkID)
	if err != nil {
		panic(err)
	}

	hostname := "relay-" + name

	driverName := "driver-" + name

	var links []string

	if name == "beta" {
		links = []string{"relay-alpha:relay-alpha"}
	}

	links = append(links, fmt.Sprintf("%s:%s", driverName, driverName))

	fmt.Println(">>>>> hostname:", hostname)

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: hostname,
		Image:    RelayServerImage,
		Tty:      false,
		Env: []string{
			"DEBUG=true",
			"RELAY_CONFIG=/opt/relay/config/server.toml",
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
	if err != nil {
		panic(err)
	}

	cli.NetworkConnect(context.Background(), p.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: p.NetworkID,
	})

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	// statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	// select {
	// case err := <-errCh:
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// case <-statusCh:
	// }
	//
	// out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
	// if err != nil {
	// 	panic(err)
	// }
	//
	// stdcopy.StdCopy(os.Stdout, os.Stderr, out)
}

func (p *Platform) RunRelayFabricDriver(
	networkName,
	relayHost, relayPort,
	driverHost, driverPort,
	interopChaincode,
	ccpPath, configPath, walletPath string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	hostname := "driver-" + networkName

	net, err := p.DockerClient.NetworkInfo(p.NetworkID)
	if err != nil {
		panic(err)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: hostname,
		Image:    FabricDriverImager,
		Tty:      false,
		Env: []string{
			"NETWORK_NAME=" + networkName,
			"RELAY_ENDPOINT=" + relayHost + ":" + relayPort,
			"DRIVER_ENDPOINT=" + driverHost + ":" + driverPort,
			"DRIVER_CONFIG=/config.json",
			"CONNECTION_PROFILE=/ccp.json",
			"INTEROP_CHAINCODE=" + interopChaincode,
			"local=false",
		},
		Cmd: []string{
			"npm", "run", "dev", "--verbose=true",
		},
		ExposedPorts: nat.PortSet{
			nat.Port(driverPort + "/tcp"): struct{}{},
			nat.Port(relayPort + "/tcp"):  struct{}{},
		},
	}, &container.HostConfig{
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
	if err != nil {
		panic(err)
	}

	cli.NetworkConnect(context.Background(), p.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: p.NetworkID,
	})

	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}

	// statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	// select {
	// case err := <-errCh:
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// case <-statusCh:
	// }
	//
	// out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true})
	// if err != nil {
	// 	panic(err)
	// }
	//
	// stdcopy.StdCopy(os.Stdout, os.Stderr, out)
}
