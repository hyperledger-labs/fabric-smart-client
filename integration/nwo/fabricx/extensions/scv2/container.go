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
	"github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapio"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/credentials"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	fabricx_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// containerName is the name of the committer container; it is unique per network.
func (e *Extension) containerName() string {
	return fmt.Sprintf("%s-%s-%s-committer", e.network.NetworkID, e.cfg.SidecarOrg, e.cfg.SidecarName)
}

// ports returns the host ports of the sidecar, query, and ordering services
// the committer container exposes.
func (e *Extension) ports() (sidecar, query, ordering int) {
	sidecarPeer := e.network.Peer(e.cfg.SidecarOrg, e.cfg.SidecarName)
	return int(e.network.PeerPort(sidecarPeer, fabric_network.ListenPort)),
		int(e.network.PeerPort(sidecarPeer, fabricx_network.QueryServicePortName)),
		int(e.network.OrdererPort(e.network.Orderers[0], fabric_network.ListenPort))
}

// committerProcesses matches the committer services the image's run script
// starts for its committer operation: verifier, vc, query, coordinator, and
// sidecar. The brackets keep the pattern from matching the shell that runs it.
const committerProcesses = "$BINS_PATH/[c]ommitter start"

// StopCommitter stops the committer services in the committer container and
// returns once they have exited. The container's database and mock orderer keep
// running: the mock orderer of the committer test container holds its blocks in
// memory only, so restarting it would lose the blocks the sidecar resumes from.
// For the same reason nwo offers no way to restart the Fabric-x orderer.
func (e *Extension) StopCommitter() {
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(d.Exec(e.containerName(), "sh", "-c", fmt.Sprintf(
		`pkill -f "%[1]s"; while pgrep -f "%[1]s" >/dev/null; do sleep 0.1; done`, committerProcesses,
	))).To(gomega.Succeed())
}

// StartCommitter starts the committer services stopped by StopCommitter and
// returns once its sidecar, query, and ordering services accept connections.
// The services resume from their database, and the sidecar fetches the blocks
// it has not processed yet from the mock orderer, so a transaction ordered after
// StartCommitter returns commits. The test fails at once if the services exit
// before they are ready.
func (e *Extension) StartCommitter() {
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// The run script waits for the services it starts, so its output stream ends
	// once they have all exited.
	created, err := d.Client.ExecCreate(ctx, e.containerName(), dcli.ExecCreateOptions{
		Cmd:          containerCmd(containerConfig{TLSEnabled: e.network.TLSEnabled}, "committer"),
		TTY:          true,
		AttachStdout: true,
		AttachStderr: true,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	attached, err := d.Client.ExecAttach(ctx, created.ID, dcli.ExecAttachOptions{TTY: true})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	e.waitUntilReady(e.forwardLogs(attached.Reader, attached.Close))
}

func (e *Extension) launchContainer() {
	logger.Infof("Launch container")

	networkID := e.network.NetworkID
	sidecarPeer := e.network.Peer(e.cfg.SidecarOrg, e.cfg.SidecarName)
	containerName := e.containerName()
	rootCryptoDir := rootCrypto(e.network)

	sidecarPort, queryServicePort, orderingServicePort := e.ports()

	// genesis block
	configBlockPath := e.network.OutputBlockPath(e.channel.Name)

	// mock orderer config
	// Temporary fix - committer v1.0.0-alpha.1 has a bug https://github.com/hyperledger/fabric-x-committer/issues/567
	// that prevents us from setting the mock orderer msp via environment variables. For that reason we create a mock
	// orderer config file and load it into the container.
	// This can be removed and replaced with proper configuration via env vars once the issue #567 is fixed.
	mockOrdererConfigPath := filepath.Clean(filepath.Join(e.network.Context.RootDir(), e.network.Prefix, "mock-orderer.yaml"))
	gomega.Expect(generateMockOrdererConfigFile(mockOrdererConfigPath, ordererConsenters(e.network))).NotTo(gomega.HaveOccurred())

	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	localIP, err := d.LocalIP(networkID)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// prep extra hosts:
	var extraHosts []string
	if runtime.GOOS == "linux" {
		extraHosts = append(extraHosts, "host.docker.internal:host-gateway")
	}

	cfg := containerConfig{
		ChannelName:           e.channel.Name,
		SidecarMSPDir:         containerSidecarMSPDir(e.network, sidecarPeer),
		SidecarMSPID:          fmt.Sprintf("%sMSP", e.cfg.SidecarOrg),
		SidecarServerEndpoint: net.JoinHostPort("", strconv.Itoa(sidecarPort)),
		QueryServerEndpoint:   net.JoinHostPort("", strconv.Itoa(queryServicePort)),
		OrdererServerEndpoint: net.JoinHostPort("", strconv.Itoa(orderingServicePort)),
		TLSEnabled:            e.network.TLSEnabled,
		CertsBundle:           path.Join("/root/artifacts/crypto", "ca-certs.pem"),
		SidecarTLSDir:         containerSidecarTLSDir(e.network, sidecarPeer),
		OrdererTLSDir:         containerOrdererTLSDir(e.network, e.network.Orderers[0]),
		Image:                 e.cfg.Image,
		EnvVarOverrides:       e.cfg.EnvVars,
	}

	logger.Infof("Run fabric-x committer test container on %v ports: sidecar=%v query=%v orderer=%v",
		localIP, sidecarPort, queryServicePort, orderingServicePort)

	cli, err := dcli.New(dcli.FromEnv)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ctx := context.TODO()
	resp, err := cli.ContainerCreate(
		ctx,
		dcli.ContainerCreateOptions{
			Name: containerName,
			Config: &container.Config{
				Image:        containerImage(cfg),
				Tty:          true,
				AttachStdout: true,
				AttachStderr: true,
				ExposedPorts: docker.PortSet(sidecarPort, queryServicePort, orderingServicePort),
				Env:          containerEnvVars(cfg),
				Cmd:          containerCmd(cfg, "db", "orderer", "committer"),
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
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	_, err = cli.ContainerStart(ctx, resp.ID, dcli.ContainerStartOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	logs, err := cli.ContainerLogs(ctx, resp.ID, dcli.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	e.waitUntilReady(e.forwardLogs(logs, func() { _ = logs.Close() }))
}

// forwardLogs forwards what the committer container writes to r to the logger
// until r ends, then calls closeReader. The returned context is canceled once r
// ends, which happens when the processes writing to it exit.
func (e *Extension) forwardLogs(r io.Reader, closeReader func()) context.Context {
	containerName := e.containerName()
	ctx, cancel := context.WithCancel(context.Background())
	dockerLogger := logging.MustGetLogger("sc.container." + containerName)
	go func() {
		defer cancel()
		dockerLogger.Debugf("fetch logs from container [%s]", containerName)
		defer dockerLogger.Debugf("stopped container log fetcher [%s], ", containerName)
		defer closeReader()

		w := &zapio.Writer{
			Log:   dockerLogger.Zap(),
			Level: zap.DebugLevel,
		}
		_, copyErr := io.Copy(w, r)
		if copyErr != nil && !errors.Is(copyErr, io.EOF) && !errors.Is(copyErr, io.ErrClosedPipe) {
			dockerLogger.Error(copyErr)
		}
	}()
	return ctx
}

// waitUntilReady waits until the sidecar, query, and ordering services of the
// committer container accept connections. The test fails if they do not within
// 90 seconds or ctx ends first.
func (e *Extension) waitUntilReady(ctx context.Context) {
	sidecarPeer := e.network.Peer(e.cfg.SidecarOrg, e.cfg.SidecarName)
	sidecarPort, queryServicePort, orderingServicePort := e.ports()

	var tlsConfig credentials.TransportCredentials
	if e.network.TLSEnabled {
		caCert, err := os.ReadFile(e.network.CACertsBundlePath())
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsDir := e.network.PeerUserTLSDir(sidecarPeer, "Admin")
		cert, err := tls.LoadX509KeyPair(
			filepath.Join(tlsDir, "client.crt"),
			filepath.Join(tlsDir, "client.key"),
		)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		tlsConfig = credentials.NewTLS(&tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{cert},
		})
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)
	for _, p := range []int{sidecarPort, orderingServicePort, queryServicePort} {
		g.Go(func() error {
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(p))
			return fabric.WaitUntilReadyWithTLS(ctx, addr, tlsConfig)
		})
	}
	gomega.Expect(g.Wait()).NotTo(gomega.HaveOccurred())
}
