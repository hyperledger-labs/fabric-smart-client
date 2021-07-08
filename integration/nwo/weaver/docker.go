/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

func RunRelayServer(name, serverConfigPath, port string) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "dlt-interop/relay-server:latest",
		Tty:   false,
		Env: []string{
			"DEBUG=true",
			"RELAY_CONFIG=/opt/relay/config/server.toml",
		},
		ExposedPorts: nat.PortSet{
			nat.Port(port + "/tcp"): struct{}{},
		},
	}, &container.HostConfig{
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
					HostIP:   "0.0.0.0",
					HostPort: port,
				},
			},
		},
	}, nil, nil, "relay-server"+name)
	if err != nil {
		panic(err)
	}

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

func RunRelayFabricDriver(
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

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: "fabric-driver:latest",
		Tty:   false,
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
			nat.Port(relayPort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: relayPort,
				},
			},
			nat.Port(driverPort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: driverPort,
				},
			},
		},
	}, nil, nil, "relay-fabric-driver-"+networkName)
	if err != nil {
		panic(err)
	}

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
