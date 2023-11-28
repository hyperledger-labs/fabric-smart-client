/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hle

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
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	. "github.com/onsi/gomega"
)

const (
	ExplorerDB = "hyperledger/explorer-db:latest"
	Explorer   = "hyperledger/explorer:latest"
)

var RequiredImages = []string{
	ExplorerDB,
	Explorer,
}

func (n *Extension) startContainer() {
	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	err = d.CheckImagesExist(RequiredImages...)
	Expect(err).NotTo(HaveOccurred())

	logger.Infof("Run Explorer DB...")
	n.startExplorerDB()
	logger.Infof("Run Explorer DB...done!")

	time.Sleep(30 * time.Second)

	logger.Infof("Run Explorer...")
	n.startExplorer()
	logger.Infof("Run Explorer...done!")
}

func (n *Extension) startExplorerDB() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	net, err := d.Client.NetworkInfo(n.platform.NetworkID())
	Expect(err).ToNot(HaveOccurred())

	containerName := n.platform.NetworkID() + "-explorerdb.mynetwork.com"

	pgdataVolumeName := n.platform.NetworkID() + "-pgdata"
	_, err = cli.VolumeCreate(ctx, volume.CreateOptions{
		Name: pgdataVolumeName,
	})
	Expect(err).ToNot(HaveOccurred())

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: "explorerdb.mynetwork.com",
		Image:    "hyperledger/explorer-db:latest",
		Tty:      false,
		Env: []string{
			"DATABASE_DATABASE=fabricexplorer",
			"DATABASE_USERNAME=hppoc",
			"DATABASE_PASSWORD=password",
		},
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: pgdataVolumeName,
				Target: "/var/lib/postgresql/data",
			},
		},
	}, &network.NetworkingConfig{
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

	dockerLogger := flogging.MustGetLogger("monitoring.hle.db.container")
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

func (n *Extension) startExplorer() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	d, err := docker.GetInstance()
	Expect(err).NotTo(HaveOccurred())

	net, err := d.Client.NetworkInfo(n.platform.NetworkID())
	Expect(err).ToNot(HaveOccurred())

	containerName := n.platform.NetworkID() + "-explorer.mynetwork.com"

	walletStoreVolumeName := n.platform.NetworkID() + "-walletstore"
	_, err = cli.VolumeCreate(ctx, volume.CreateOptions{
		Name: walletStoreVolumeName,
	})
	Expect(err).ToNot(HaveOccurred())

	localIP, err := d.LocalIP(n.platform.NetworkID())
	Expect(err).ToNot(HaveOccurred())

	port := strconv.Itoa(n.platform.HyperledgerExplorerPort())
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Hostname: "explorer.mynetwork.com",
		Image:    "hyperledger/explorer:latest",
		Tty:      false,
		Env: []string{
			"DATABASE_HOST=explorerdb.mynetwork.com",
			"DATABASE_DATABASE=fabricexplorer",
			"DATABASE_USERNAME=hppoc",
			"DATABASE_PASSWD=password",
			"LOG_LEVEL_APP=debug",
			"LOG_LEVEL_DB=debug",
			"LOG_LEVEL_CONSOLE=debug",
			"LOG_CONSOLE_STDOUT=true",
			"DISCOVERY_AS_LOCALHOST=false",
		},
		// Healthcheck: &container.HealthConfig{
		// 	Test:        []string{"pg_isready", "-h", "localhost", "-p", "5432", "-q", "-U", "postgres"},
		// 	Interval:    30 * time.Second,
		// 	Timeout:     10 * time.Second,
		// 	Retries:     5,
		// },
		ExposedPorts: nat.PortSet{
			nat.Port(port + "/tcp"): struct{}{},
		},
	}, &container.HostConfig{
		ExtraHosts: []string{fmt.Sprintf("fabric:%s", localIP)},
		Links:      []string{n.platform.NetworkID() + "-explorerdb.mynetwork.com"},
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: n.configFilePath(),
				Target: "/opt/explorer/app/platform/fabric/config.json",
			},
			{
				Type:   mount.TypeBind,
				Source: n.cpFileDir(),
				Target: "/opt/explorer/app/platform/fabric/connection-profile",
			},
			// {
			// 	Type:   mount.TypeBind,
			// 	Source: n.platform.CryptoPath(),
			// 	Target: "/tmp/crypto",
			// },
			{
				Type:   mount.TypeVolume,
				Source: walletStoreVolumeName,
				Target: "/opt/explorer/wallet",
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

	dockerLogger := flogging.MustGetLogger("monitoring.hle.container")
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
