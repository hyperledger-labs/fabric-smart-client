/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapio"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/credentials"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	fabricx_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

func (e *Extension) launchContainer() {
	logger.Infof("Launch container")

	networkID := e.network.NetworkID
	orgName := e.network.PeerOrgs()[0].Name
	sidecarPeer := e.network.Peer(orgName, e.sidecar.Name)
	containerName := fmt.Sprintf("%s-%s-scalable-committer", networkID, orgName)
	rootCryptoDir := rootCrypto(e.network)

	// get ports
	sidecarPort := int(e.network.PeerPort(sidecarPeer, fabric_network.ListenPort))
	queryServicePort := int(e.network.PeerPort(sidecarPeer, fabricx_network.QueryServicePortName))
	orderingServicePort := int(e.network.OrdererPort(e.network.Orderers[0], fabric_network.ListenPort))

	// genesis block
	configBlockPath := e.network.OutputBlockPath(e.channel.Name)

	// mock orderer config
	// Temporary fix - committer v1.0.0-alpha.1 has a bug https://github.com/hyperledger/fabric-x-committer/issues/567
	// that prevents us from setting the mock orderer msp via environment variables. For that reason we create a mock
	// orderer config file and load it into the container.
	// This can be removed and replaced with proper configuration via env vars once the issue #567 is fixed.
	mockOrdererConfigPath := filepath.Clean(filepath.Join(e.network.Context.RootDir(), e.network.Prefix, "mock-orderer.yaml"))
	err := generateMockOrdererConfigFile(mockOrdererConfigPath)
	utils.Must(err)

	d, err := docker.GetInstance()
	utils.Must(err)

	netInfo, err := d.NetworkInfo(networkID)
	utils.Must(err)
	logger.Infof("netInfo id: %s", netInfo.ID)

	localIP, err := d.LocalIP(networkID)
	utils.Must(err)

	// prep extra hosts:
	var extraHosts []string
	if runtime.GOOS == "linux" {
		extraHosts = append(extraHosts, "host.docker.internal:host-gateway")
	}

	cfg := containerConfig{
		ChannelName:           e.channel.Name,
		SidecarMSPDir:         containerSidecarMSPDir(e.network, sidecarPeer),
		SidecarMSPID:          fmt.Sprintf("%sMSP", orgName),
		SidecarServerEndpoint: net.JoinHostPort("", strconv.Itoa(sidecarPort)),
		QueryServerEndpoint:   net.JoinHostPort("", strconv.Itoa(queryServicePort)),
		OrdererServerEndpoint: net.JoinHostPort("", strconv.Itoa(orderingServicePort)),
		TLSEnabled:            e.network.TLSEnabled,
		CertsBundle:           path.Join("/root/artifacts/crypto", "ca-certs.pem"),
		SidecarTLSDir:         containerSidecarTLSDir(e.network, sidecarPeer),
		OrdererTLSDir:         containerOrdererTLSDir(e.network, e.network.Orderers[0]),
	}

	logger.Infof("Run fabric-x committer test container on %v ports: sidecar=%v query=%v orderer=%v",
		localIP, sidecarPort, queryServicePort, orderingServicePort)

	cli, err := dcli.New(dcli.FromEnv)
	utils.Must(err)
	ctx := context.TODO()
	resp, err := cli.ContainerCreate(
		ctx,
		dcli.ContainerCreateOptions{
			Name: containerName,
			Config: &container.Config{
				Image:        scalableCommitterImage,
				Tty:          true,
				AttachStdout: true,
				AttachStderr: true,
				ExposedPorts: docker.PortSet(sidecarPort, queryServicePort, orderingServicePort),
				Env:          containerEnvVars(cfg),
				Cmd:          containerCmd(cfg),
			},
			HostConfig: &container.HostConfig{
				ExtraHosts: extraHosts,
				Mounts: []mount.Mount{
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
					{ // custom orderer config
						Type:     mount.TypeBind,
						Source:   mockOrdererConfigPath,
						Target:   "/root/config/mock-orderer.yaml",
						ReadOnly: true,
					},
				},
				PortBindings: docker.PortBindings(sidecarPort, queryServicePort, orderingServicePort),
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

		// copy returns when the container is stopped
		// and therefore cancels the context
		_, copyErr := io.Copy(w, reader)
		if copyErr != nil && !errors.Is(copyErr, io.EOF) && !errors.Is(copyErr, io.ErrClosedPipe) {
			dockerLogger.Error(copyErr)
		}
	}()

	var tlsConfig credentials.TransportCredentials
	if e.network.TLSEnabled {
		caCert, err := os.ReadFile(e.network.CACertsBundlePath())
		utils.Must(err)
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsDir := e.network.PeerUserTLSDir(sidecarPeer, "Admin")
		cert, err := tls.LoadX509KeyPair(
			filepath.Join(tlsDir, "client.crt"),
			filepath.Join(tlsDir, "client.key"),
		)
		utils.Must(err)

		tlsConfig = credentials.NewTLS(&tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{cert},
		})
	}

	const timeout = 90 * time.Second
	// we extend container logging context with a timeout;
	// if the container is closed or the timeout fires;
	// the context is canceled and aborts the wait function
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	// let's wait until the sidecar is ready
	g, ctx := errgroup.WithContext(ctx)
	for _, p := range []int{sidecarPort, orderingServicePort, queryServicePort} {
		g.Go(func() error {
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(p))
			return fabric.WaitUntilReadyWithTLS(ctx, addr, tlsConfig)
		})
	}
	err = g.Wait()
	utils.Must(err)
}
