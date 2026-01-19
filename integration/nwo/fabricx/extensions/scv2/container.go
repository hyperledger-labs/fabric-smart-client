/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	docker_network "github.com/docker/docker/api/types/network"
	docker_client "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	v3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapio"
)

var (
	scalableCommitterImages = map[string]string{
		v3.CommitterVersion: v3.ScalableCommitterImage,
	}
	envVars = map[string]func(peerMSPDir, scMSPID, channelName, ordererEndpoint string) []string{
		v3.CommitterVersion: v3.ContainerEnvVars,
	}
	containerCmds = map[string][]string{
		// note that v1 and v2 don't use any specified cmds
		v3.CommitterVersion: v3.ContainerCmd,
	}
	sidecarDefaultPort = map[string]nat.Port{
		v3.CommitterVersion: v3.SidecarDefaultPort,
	}
	queryServiceDefaultPort = map[string]nat.Port{
		v3.CommitterVersion: v3.QueryServiceDefaultPort,
	}

	committerVersion = v3.CommitterVersion
)

const (
	queryServiceConfig = `query-service:
  server:
    endpoint:
      host: localhost
      port: 7001
    keep-alive:
      params:
        time: 60s
        timeout 600s
  database:
    host: localhost
    port: 5433
    username: yugabyte
    password: yugabyte
    database: yugabyte
    max-connections: 10
    min-connections: 5
  max-batch-wait: 0.5ms
  min-batch-keys: 1
  monitoring:
    metrics:
      enable: true
      endpoint: localhost:2114
logging:
  development: true
  enabled: true
  level: DEBUG
`
)

func dockerHostAlias() string {
	return "host.docker.internal"
}

func (e *Extension) launchContainer() {
	logger.Infof("Launch container")

	networkID := e.network.NetworkID
	orgName := e.network.PeerOrgs()[0].Name
	scPeer := e.network.Peer(orgName, e.sidecar.Name)
	sidecarPort := fmt.Sprintf("%d", e.network.PeerPort(scPeer, fabric_network.ListenPort))

	// TODO: get this via network config
	queryServicePort := "7001"
	orderingServicePort := fmt.Sprintf("%d", e.network.OrdererPort(e.network.Orderers[0], fabric_network.ListenPort))

	logger.Infof("Sidecar running on port [%s]", sidecarPort)
	containerName := fmt.Sprintf("%s-scalable-committer", networkID)
	orderer := e.network.Orderer("orderer")
	ordererEndpoint := fmt.Sprintf("%s:%d", dockerHostAlias(), e.network.OrdererPort(orderer, fabric_network.ListenPort))
	scMSPID := fmt.Sprintf("%sMSP", orgName)

	// create queryservice config
	configDir := filepath.Join(e.network.RootDir, e.network.Prefix, "config")
	err := os.MkdirAll(configDir, 0o750)
	utils.Must(err)
	qsConfigPath := filepath.Join(configDir, "config-queryservice.yaml")
	err = os.WriteFile(qsConfigPath, []byte(queryServiceConfig), 0o644)
	utils.Must(err)

	rootCryptoDir := rootCrypto(e.network)
	peerMSPDir := peerDockerMSPDir(e.network, scPeer)

	// genesis block
	configBlockPath := e.network.OutputBlockPath(e.channel.Name)

	// TODO: remove this old docker dino
	d, err := docker.GetInstance()
	utils.Must(err)

	net, err := d.Client.NetworkInfo(networkID)
	utils.Must(err)
	logger.Infof("net id: %s", net.ID)

	localIP, err := d.LocalIP(networkID)
	utils.Must(err)

	// prep extra hosts:
	var extraHosts []string
	if runtime.GOOS == "linux" {
		extraHosts = append(extraHosts, "host.docker.internal:host-gateway")
	}

	containerEnvOverride := envVars[committerVersion](peerMSPDir, scMSPID, e.channel.Name, ordererEndpoint)
	containerCmd := containerCmds[committerVersion]
	containerSidecarPort := sidecarDefaultPort[committerVersion]
	containerQueryServicePort := queryServiceDefaultPort[committerVersion]

	logger.Infof("Run Scalable Committer container on %v ports: %v %v\ncontainer env vars: %v", localIP, sidecarPort, queryServicePort, containerEnvOverride)
	defer logger.Infof("Run Scalable Committer container on port [%s]...done", sidecarPort)

	cli, err := docker_client.NewClientWithOpts(docker_client.FromEnv, docker_client.WithAPIVersionNegotiation())
	utils.Must(err)

	ctx := context.Background()
	resp, err := cli.ContainerCreate(
		ctx,
		&container.Config{
			Image:        scalableCommitterImages[committerVersion],
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
			ExposedPorts: nat.PortSet{
				nat.Port(sidecarPort + "/tcp"):         struct{}{},
				nat.Port(queryServicePort + "/tcp"):    struct{}{},
				nat.Port(orderingServicePort + "/tcp"): struct{}{},
			},
			Env: containerEnvOverride,
			Cmd: containerCmd,
		},
		&container.HostConfig{
			ExtraHosts: extraHosts,
			Mounts: []mount.Mount{
				{
					// crypto
					Type:   mount.TypeBind,
					Source: rootCryptoDir,
					Target: "/root/config/crypto",
				},
				{
					// query service config
					Type:   mount.TypeBind,
					Source: qsConfigPath,
					Target: "/root/config/config-queryservice.yaml",
				},
				{ // config block
					Type:     mount.TypeBind,
					Source:   configBlockPath,
					Target:   "/root/config/sc-genesis-block.proto.bin",
					ReadOnly: true,
				},
			},
			PortBindings: nat.PortMap{
				// sidecar port binding
				containerSidecarPort: []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: sidecarPort,
					},
				},
				// query service port bindings
				containerQueryServicePort: []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: queryServicePort,
					},
				},
				// sidecar port binding
				nat.Port(orderingServicePort + "/tcp"): []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: orderingServicePort,
					},
				},
			},
		},
		&docker_network.NetworkingConfig{
			EndpointsConfig: map[string]*docker_network.EndpointSettings{
				networkID: {},
			},
		},
		nil, containerName,
	)
	utils.Must(err)

	err = cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
	utils.Must(err)

	ctx, cancel := context.WithCancel(context.Background())
	dockerLogger := logging.MustGetLogger("sc.container." + resp.ID[:8])
	go func() {
		defer cancel()
		dockerLogger.Debugf("fetch logs from container [%s]", containerName)
		defer dockerLogger.Debugf("stopped container log fetcher [%s], ", containerName)

		reader, errx := cli.ContainerLogs(context.Background(), resp.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		utils.Must(errx)
		defer func() {
			_ = reader.Close()
		}()

		w := &zapio.Writer{
			Log:   dockerLogger.Zap(),
			Level: zap.DebugLevel,
		}

		_, err = io.Copy(w, reader)
		if err != nil && errors.Is(err, io.EOF) {
			dockerLogger.Fatal(err)
		}
	}()

	// let's wait until the sidecar is ready
	sidecarEndpoint := fmt.Sprintf("%s:%s", localIP, sidecarPort)
	timeout := 90 * time.Second

	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	logger.Infof("Checking sidecar health-check at %v", sidecarEndpoint)
	err = fabric.WaitUntilReady(ctx, sidecarEndpoint)
	utils.Must(err)

	time.Sleep(1 * time.Second)
}
