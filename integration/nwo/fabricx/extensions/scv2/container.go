/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapio"
	"google.golang.org/grpc/credentials"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	v3 "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/extensions/v3"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

var (
	scalableCommitterImages = map[string]string{
		v3.CommitterVersion: v3.ScalableCommitterImage,
	}
	envVars = map[string]func(scMSPDir, scTLSDir, scMSPID, channelName, ordererEndpoint string, tlsEnabled bool, ordererTLSCACert string) []string{
		v3.CommitterVersion: v3.ContainerEnvVars,
	}
	containerCmds = map[string]func(tlsEnabled bool) []string{
		// note that v1 and v2 don't use any specified cmds
		v3.CommitterVersion: v3.ContainerCmd,
	}
	sidecarDefaultPort = map[string]network.Port{
		v3.CommitterVersion: network.MustParsePort(v3.SidecarDefaultPort),
	}
	queryServiceDefaultPort = map[string]network.Port{
		v3.CommitterVersion: network.MustParsePort(v3.QueryServiceDefaultPort),
	}

	committerVersion = v3.CommitterVersion
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

	rootCryptoDir := rootCrypto(e.network)
	scMSPDir := scDockerMSPDir(e.network, scPeer)
	scTLSDir := scDockerTLSDir(e.network, scPeer)

	// genesis block
	configBlockPath := e.network.OutputBlockPath(e.channel.Name)

	// TODO: remove this old docker dino
	d, err := docker.GetInstance()
	utils.Must(err)

	net, err := d.NetworkInfo(networkID)
	utils.Must(err)
	logger.Infof("net id: %s", net.ID)

	localIP, err := d.LocalIP(networkID)
	utils.Must(err)

	// prep extra hosts:
	var extraHosts []string
	if runtime.GOOS == "linux" {
		extraHosts = append(extraHosts, "host.docker.internal:host-gateway")
	}

	containerEnvOverride := envVars[committerVersion](scMSPDir, scTLSDir, scMSPID, e.channel.Name, ordererEndpoint, e.network.TLSEnabled, path.Join(scTLSDir, "ca.crt"))
	containerCmd := containerCmds[committerVersion](e.network.TLSEnabled)
	containerSidecarPort := sidecarDefaultPort[committerVersion]
	containerQueryServicePort := queryServiceDefaultPort[committerVersion]

	logger.Infof("Run Scalable Committer container on %v ports: %v %v\ncontainer env vars: %v", localIP, sidecarPort, queryServicePort, containerEnvOverride)
	defer logger.Infof("Run Scalable Committer container on port [%s]...done", sidecarPort)

	cli, err := dcli.New(dcli.FromEnv)
	utils.Must(err)

	ctx := context.TODO()
	resp, err := cli.ContainerCreate(
		ctx,
		dcli.ContainerCreateOptions{
			Name: containerName,
			Config: &container.Config{
				Image:        scalableCommitterImages[committerVersion],
				Tty:          true,
				AttachStdout: true,
				AttachStderr: true,
				ExposedPorts: network.PortSet{
					network.MustParsePort(sidecarPort + "/tcp"):         struct{}{},
					network.MustParsePort(queryServicePort + "/tcp"):    struct{}{},
					network.MustParsePort(orderingServicePort + "/tcp"): struct{}{},
				},
				Env: containerEnvOverride,
				Cmd: containerCmd,
			},
			HostConfig: &container.HostConfig{
				ExtraHosts: extraHosts,
				Mounts: append([]mount.Mount{
					{
						// crypto
						Type:   mount.TypeBind,
						Source: rootCryptoDir,
						Target: "/root/artifacts/crypto",
					},
					{ // config block
						Type:     mount.TypeBind,
						Source:   configBlockPath,
						Target:   "/root/artifacts/config-block.pb.bin",
						ReadOnly: true,
					},
				}, func() []mount.Mount {
					if !e.network.TLSEnabled {
						return nil
					}
					hostTLSDir := e.network.PeerLocalTLSDir(scPeer)
					return []mount.Mount{
						{Type: mount.TypeBind, Source: filepath.Join(hostTLSDir, "server.crt"), Target: "/server-certs/public-key.pem"},
						{Type: mount.TypeBind, Source: filepath.Join(hostTLSDir, "server.key"), Target: "/server-certs/private-key.pem"},
						{Type: mount.TypeBind, Source: filepath.Join(hostTLSDir, "ca.crt"), Target: "/server-certs/ca-certificate.pem"},
						{Type: mount.TypeBind, Source: filepath.Join(hostTLSDir, "server.crt"), Target: "/client-certs/public-key.pem"},
						{Type: mount.TypeBind, Source: filepath.Join(hostTLSDir, "server.key"), Target: "/client-certs/private-key.pem"},
						{Type: mount.TypeBind, Source: filepath.Join(hostTLSDir, "ca.crt"), Target: "/client-certs/ca-certificate.pem"},
					}
				}()...),
				PortBindings: network.PortMap{
					// sidecar port binding
					containerSidecarPort: []network.PortBinding{
						{
							HostIP:   netip.MustParseAddr("0.0.0.0"),
							HostPort: sidecarPort,
						},
					},
					// query service port bindings
					containerQueryServicePort: []network.PortBinding{
						{
							HostIP:   netip.MustParseAddr("0.0.0.0"),
							HostPort: queryServicePort,
						},
					},
					// sidecar port binding
					network.MustParsePort(orderingServicePort + "/tcp"): []network.PortBinding{
						{
							HostIP:   netip.MustParseAddr("0.0.0.0"),
							HostPort: orderingServicePort,
						},
					},
				},
			},
			NetworkingConfig: &network.NetworkingConfig{
				EndpointsConfig: map[string]*network.EndpointSettings{
					networkID: {},
				},
			},
		},
	)
	utils.Must(err)

	_, err = cli.ContainerStart(ctx, resp.ID, dcli.ContainerStartOptions{})
	utils.Must(err)

	ctx, cancel := context.WithCancel(context.TODO())
	dockerLogger := logging.MustGetLogger("sc.container." + resp.ID[:8])
	go func() {
		defer cancel()
		dockerLogger.Debugf("fetch logs from container [%s]", containerName)
		defer dockerLogger.Debugf("stopped container log fetcher [%s], ", containerName)

		reader, errx := cli.ContainerLogs(context.TODO(), resp.ID, dcli.ContainerLogsOptions{
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
	var tlsConfig credentials.TransportCredentials
	if e.network.TLSEnabled {
		caCertPath := filepath.Join(e.network.PeerLocalTLSDir(scPeer), "ca.crt")
		caCert, err := os.ReadFile(caCertPath)
		utils.Must(err)
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		cert, err := tls.LoadX509KeyPair(
			filepath.Join(e.network.PeerLocalTLSDir(scPeer), "server.crt"),
			filepath.Join(e.network.PeerLocalTLSDir(scPeer), "server.key"),
		)
		utils.Must(err)

		tlsConfig = credentials.NewTLS(&tls.Config{
			RootCAs:            caCertPool,
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: true,
		})
	}

	err = fabric.WaitUntilReadyWithTLS(ctx, sidecarEndpoint, tlsConfig)
	if err != nil {
		// dump logs
		logger.Errorf("healthcheck failed for container [%s]: %v", containerName, err)
		reader, errLog := cli.ContainerLogs(context.Background(), resp.ID, dcli.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
		})
		if errLog == nil {
			payload, errRead := io.ReadAll(reader)
			if errRead == nil {
				logger.Errorf("Container [%s] logs:\n%s", containerName, string(payload))
			}
			_ = reader.Close()
		}
		utils.Must(err)
	}

	time.Sleep(1 * time.Second)
}
