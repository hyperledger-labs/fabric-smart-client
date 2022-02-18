/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hle

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/helpers"
	nnetwork "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const (
	ExplorerDB = "hyperledger/explorer-db:latest"
	Explorer   = "hyperledger/explorer:latest"
)

var RequiredImages = []string{
	ExplorerDB,
	Explorer,
}

var logger = flogging.MustGetLogger("integration.nwo.fabric.hle")

type Platform interface {
	HyperledgerExplorer() bool
	GetContext() api.Context
	ConfigDir() string
	DockerClient() *docker.Client
	NetworkID() string
	HyperledgerExplorerPort() int
}

type Extension struct {
	platform Platform
}

func NewExtension(platform Platform) *Extension {
	return &Extension{
		platform: platform,
	}
}

func (n *Extension) CheckTopology() {
	if !n.platform.HyperledgerExplorer() {
		return
	}

	helpers.AssertImagesExist(RequiredImages...)

	return
}

func (n *Extension) GenerateArtifacts() {
	if !n.platform.HyperledgerExplorer() {
		return
	}

	config := Config{
		NetworkConfigs: map[string]Network{},
		License:        "Apache-2.0",
	}

	// Generate and store config for each fabric network
	for _, platform := range n.platform.GetContext().PlatformsByType(fabric.TopologyName) {
		fabricPlatform := platform.(*fabric.Platform)

		networkName := fmt.Sprintf("hlf-%s", fabricPlatform.Topology().Name())
		config.NetworkConfigs[fabricPlatform.Topology().Name()] = Network{
			Name:                 fmt.Sprintf("Fabric Network (%s)", networkName),
			Profile:              "./connection-profile/" + fabricPlatform.Topology().Name() + ".json",
			EnableAuthentication: true,
		}

		// marshal config
		configJSON, err := json.MarshalIndent(config, "", "  ")
		Expect(err).NotTo(HaveOccurred())
		// write config to file
		Expect(os.MkdirAll(n.configFileDir(), 0o755)).NotTo(HaveOccurred())
		Expect(ioutil.WriteFile(n.configFilePath(), configJSON, 0o644)).NotTo(HaveOccurred())

		// Generate and store connection profile
		cp := fabricPlatform.ConnectionProfile(fabricPlatform.Topology().Name(), false)
		// add client section
		cp.Client = nnetwork.Client{
			AdminCredential: nnetwork.AdminCredential{
				Id:       "admin",
				Password: "admin",
			},
			Organization:         fabricPlatform.PeerOrgs()[0].Name,
			EnableAuthentication: true,
			TlsEnable:            true,
			Connection: nnetwork.Connection{
				Timeout: nnetwork.Timeout{
					Peer: map[string]string{
						"endorser": "600",
					},
				},
			},
		}
		cpJSON, err := json.MarshalIndent(cp, "", "  ")
		Expect(err).NotTo(HaveOccurred())
		// write cp to file
		Expect(os.MkdirAll(n.cpFileDir(), 0o755)).NotTo(HaveOccurred())
		Expect(ioutil.WriteFile(n.cpFilePath(fabricPlatform.Topology().Name()), cpJSON, 0o644)).NotTo(HaveOccurred())
	}
}

func (n *Extension) PostRun(bool) {
	if !n.platform.HyperledgerExplorer() {
		return
	}

	logger.Infof("Run Explorer DB...")
	n.dockerExplorerDB()
	logger.Infof("Run Explorer DB...done!")
	time.Sleep(30 * time.Second)
	logger.Infof("Run Explorer...")
	n.dockerExplorer()
	logger.Infof("Run Explorer...done!")
}

func (n *Extension) checkTopology() {
}

func (n *Extension) dockerExplorerDB() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	net, err := n.platform.DockerClient().NetworkInfo(n.platform.NetworkID())
	Expect(err).ToNot(HaveOccurred())

	containerName := n.platform.NetworkID() + "-explorerdb.mynetwork.com"

	pgdataVolumeName := n.platform.NetworkID() + "-pgdata"
	_, err = cli.VolumeCreate(ctx, volume.VolumeCreateBody{
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

func (n *Extension) dockerExplorer() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	net, err := n.platform.DockerClient().NetworkInfo(n.platform.NetworkID())
	Expect(err).ToNot(HaveOccurred())

	containerName := n.platform.NetworkID() + "-explorer.mynetwork.com"

	walletStoreVolumeName := n.platform.NetworkID() + "-walletstore"
	_, err = cli.VolumeCreate(ctx, volume.VolumeCreateBody{
		Name: walletStoreVolumeName,
	})
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
		ExtraHosts: []string{fmt.Sprintf("fabric:%s", nnetwork.LocalIP(n.platform.DockerClient(), n.platform.NetworkID()))},
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

func (n *Extension) configFileDir() string {
	return filepath.Join(
		n.platform.ConfigDir(),
		"hle",
	)
}

func (n *Extension) configFilePath() string {
	return filepath.Join(n.configFileDir(), "config.json")
}

func (n *Extension) cpFileDir() string {
	return filepath.Join(
		n.configFileDir(),
		"connection-profile",
	)
}

func (n *Extension) cpFilePath(name string) string {
	return filepath.Join(n.cpFileDir(), name+".json")
}
