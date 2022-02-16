/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hle

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
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

type ConnectionProfileProvider interface {
	ConnectionProfile(name string, ca bool) *nnetwork.ConnectionProfile
}

type Extension struct {
	network *nnetwork.Network
	cpp     ConnectionProfileProvider
}

func NewExtension(network *nnetwork.Network, cpp ConnectionProfileProvider) *Extension {
	return &Extension{
		network: network,
		cpp:     cpp,
	}
}

func (n *Extension) CheckTopology() {
	if !n.network.Topology().HyperledgerExplorer {
		return
	}

	helpers.AssertImagesExist(RequiredImages...)

	return
}

func (n *Extension) GenerateArtifacts() {
	if !n.network.Topology().HyperledgerExplorer {
		return
	}

	// Generate and store config

	networkName := fmt.Sprintf("hlf-%s", n.network.Topology().Name())
	config := Config{
		NetworkConfigs: map[string]Network{
			"test-network": {
				Name:                 fmt.Sprintf("Fabric Network (%s)", networkName),
				Profile:              "./connection-profile/" + n.network.Topology().TopologyName + ".json",
				EnableAuthentication: true,
			},
		},
		License: "Apache-2.0",
	}
	// marshal config
	configJSON, err := json.MarshalIndent(config, "", "  ")
	Expect(err).NotTo(HaveOccurred())
	// write config to file
	Expect(os.MkdirAll(n.configFileDir(), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(n.configFilePath(), configJSON, 0o644)).NotTo(HaveOccurred())

	// Generate and store connection profile
	cp := n.cpp.ConnectionProfile(networkName, false)
	// add client section
	cp.Client = nnetwork.Client{
		AdminCredential: nnetwork.AdminCredential{
			Id:       "admin",
			Password: "admin",
		},
		Organization:         n.network.PeerOrgs()[0].Name,
		EnableAuthentication: true,
		TlsEnable:            true,
		Connection: nnetwork.Connection{
			Timeout: nnetwork.Timeout{
				Peer: map[string]string{
					"endorser": "300",
				},
			},
		},
	}
	cpJSON, err := json.MarshalIndent(cp, "", "  ")
	Expect(err).NotTo(HaveOccurred())
	// write cp to file
	Expect(os.MkdirAll(n.cpFileDir(), 0o755)).NotTo(HaveOccurred())
	Expect(ioutil.WriteFile(n.cpFilePath(), cpJSON, 0o644)).NotTo(HaveOccurred())
}

func (n *Extension) PostRun(bool) {
	if !n.network.Topology().HyperledgerExplorer {
		return
	}

	logger.Infof("Run Explorer DB [%s]...", n.network.Topology().Name())
	n.dockerExplorerDB()
	logger.Infof("Run Explorer DB [%s]...done!", n.network.Topology().Name())
	time.Sleep(30 * time.Second)
	logger.Infof("Run Explorer [%s]...", n.network.Topology().Name())
	n.dockerExplorer()
	logger.Infof("Run Explorer [%s]...done!", n.network.Topology().Name())
}

func (n *Extension) checkTopology() {
}

func (n *Extension) dockerExplorerDB() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	net, err := n.network.DockerClient.NetworkInfo(n.network.NetworkID)
	Expect(err).ToNot(HaveOccurred())

	_, err = cli.VolumeCreate(ctx, volume.VolumeCreateBody{
		Name: "pgdata",
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
				Source: "pgdata",
				Target: "/var/lib/postgresql/data",
			},
		},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			n.network.NetworkID: {
				NetworkID: net.ID,
			},
		},
	}, nil, "explorerdb.mynetwork.com",
	)
	Expect(err).ToNot(HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.network.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: n.network.NetworkID,
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})).ToNot(HaveOccurred())
}

func (n *Extension) dockerExplorer() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	Expect(err).ToNot(HaveOccurred())

	net, err := n.network.DockerClient.NetworkInfo(n.network.NetworkID)
	Expect(err).ToNot(HaveOccurred())

	_, err = cli.VolumeCreate(ctx, volume.VolumeCreateBody{
		Name: "walletstore",
	})
	Expect(err).ToNot(HaveOccurred())

	port := "8080"
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
		Links: []string{"explorerdb.mynetwork.com"},
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
			// 	Source: n.network.CryptoPath(),
			// 	Target: "/tmp/crypto",
			// },
			{
				Type:   mount.TypeVolume,
				Source: "walletstore",
				Target: "/opt/explorer/wallet",
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
	},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				n.network.NetworkID: {
					NetworkID: net.ID,
				},
			},
		}, nil, "explorer.mynetwork.com",
	)
	Expect(err).ToNot(HaveOccurred())

	err = cli.NetworkConnect(context.Background(), n.network.NetworkID, resp.ID, &network.EndpointSettings{
		NetworkID: n.network.NetworkID,
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})).ToNot(HaveOccurred())
	time.Sleep(3 * time.Second)
}

func (n *Extension) configFileDir() string {
	return filepath.Join(
		n.network.ConfigDir(),
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

func (n *Extension) cpFilePath() string {
	return filepath.Join(n.cpFileDir(), n.network.Topology().TopologyName+".json")
}
