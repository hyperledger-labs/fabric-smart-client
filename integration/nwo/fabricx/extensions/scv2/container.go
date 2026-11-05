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
	"slices"
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
	fabric_topo "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
	fabricx_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx/network"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// containerName is the name of the committer's container; it is unique per network.
func (c *committer) containerName() string {
	return fmt.Sprintf("%s-%s-%s-committer", c.ext.network.NetworkID, c.cfg.SidecarOrg, c.cfg.SidecarName)
}

// sidecarPeer is the peer whose credentials the committer's sidecar uses.
func (c *committer) sidecarPeer() *fabric_topo.Peer {
	return c.ext.network.Peer(c.cfg.SidecarOrg, c.cfg.SidecarName)
}

// ordererContainerName is the name of the mock orderer container; it is unique per network.
func (e *Extension) ordererContainerName() string {
	return fmt.Sprintf("%s-orderer", e.network.NetworkID)
}

// ports returns the host ports of the sidecar and query services the committer's
// container exposes.
func (c *committer) ports() (sidecar, query int) {
	n := c.ext.network
	return int(n.PeerPort(c.sidecarPeer(), fabric_network.ListenPort)),
		int(n.PeerPort(c.sidecarPeer(), fabricx_network.QueryServicePortName))
}

// orderingPort returns the host port of the mock orderer.
func (e *Extension) orderingPort() int {
	return int(e.network.OrdererPort(e.network.Orderers[0], fabric_network.ListenPort))
}

// committersNamed returns the committers named names, or every committer when names is
// empty. The test fails if a name matches no committer.
func (e *Extension) committersNamed(names ...string) []*committer {
	if len(names) == 0 {
		return e.committers
	}
	var cs []*committer
	for _, name := range names {
		i := slices.IndexFunc(e.committers, func(c *committer) bool { return c.cfg.SidecarName == name })
		gomega.Expect(i).NotTo(gomega.Equal(-1), "no committer named [%s]", name)
		cs = append(cs, e.committers[i])
	}
	return cs
}

// committerProcesses matches the committer services the image's run script
// starts for its committer operation: verifier, vc, query, coordinator, and
// sidecar. The brackets keep the pattern from matching the shell that runs it.
const committerProcesses = "$BINS_PATH/[c]ommitter start"

// StopCommitter stops the services of the committers named names, every committer when
// names is empty, and returns once they have exited. Each committer's database keeps
// running, and the mock orderer, which runs in a container of its own, is not touched: it
// holds its blocks in memory only, so restarting it would lose the blocks the sidecars
// resume from. For the same reason nwo offers no way to restart the Fabric-x orderer.
//
// The test fails if a name matches no committer, or if a committer has no service
// running, which is also how a change to the image that committerProcesses no longer
// matches shows up: pkill exits non-zero when it matches nothing.
func (e *Extension) StopCommitter(names ...string) {
	for _, c := range e.committersNamed(names...) {
		c.stop()
	}
}

// StartCommitter starts the services of the committers named names, every committer when
// names is empty, stopped by StopCommitter, and returns once their sidecar and query
// services accept connections. The services resume from their
// database, and each sidecar fetches the blocks it has not processed yet from the mock
// orderer, so a transaction ordered after StartCommitter returns commits. The test fails
// at once if the services exit before they are ready.
func (e *Extension) StartCommitter(names ...string) {
	for _, c := range e.committersNamed(names...) {
		c.start()
	}
}

func (c *committer) stop() {
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(d.Exec(c.containerName(), "sh", "-c", fmt.Sprintf(
		`pkill -f "%[1]s" && while pgrep -f "%[1]s" >/dev/null; do sleep 0.1; done`, committerProcesses,
	))).To(gomega.Succeed())
}

func (c *committer) start() {
	e := c.ext
	d, err := docker.GetInstance()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// The run script waits for the services it starts, so its output stream ends
	// once they have all exited.
	created, err := d.Client.ExecCreate(ctx, c.containerName(), dcli.ExecCreateOptions{
		Cmd:          containerCmd(containerConfig{TLSEnabled: e.network.TLSEnabled}, "committer"),
		TTY:          true,
		AttachStdout: true,
		AttachStderr: true,
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	attached, err := d.Client.ExecAttach(ctx, created.ID, dcli.ExecAttachOptions{TTY: true})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	sidecarPort, queryServicePort := c.ports()
	e.waitUntilReady(e.forwardLogs(c.containerName(), attached.Reader, attached.Close), sidecarPort, queryServicePort)
}

func (e *Extension) launchContainers() {
	logger.Infof("Launch containers")

	orderingServicePort := e.orderingPort()

	// genesis block
	configBlockPath := e.network.OutputBlockPath(e.channel.Name)

	// mock orderer config
	// Temporary fix - committer v1.0.0-alpha.1 has a bug https://github.com/hyperledger/fabric-x-committer/issues/567
	// that prevents us from setting the mock orderer msp via environment variables. For that reason we create a mock
	// orderer config file and load it into the container.
	// This can be removed and replaced with proper configuration via env vars once the issue #567 is fixed.
	mockOrdererConfigPath := filepath.Clean(filepath.Join(e.network.Context.RootDir(), e.network.Prefix, "mock-orderer.yaml"))
	gomega.Expect(generateMockOrdererConfigFile(mockOrdererConfigPath, ordererConsenters(e.network))).NotTo(gomega.HaveOccurred())

	crypto := mount.Mount{Type: mount.TypeBind, Source: rootCrypto(e.network), Target: "/root/artifacts/crypto"}
	block := mount.Mount{Type: mount.TypeBind, Source: configBlockPath, Target: "/root/artifacts/config-block.pb.bin", ReadOnly: true}

	// The mock orderer runs in a container of its own, so that a committer can be stopped
	// while the orderer keeps the blocks every other committer reads, and so that each
	// organization can run a committer of its own against the one orderer. Committers reach
	// it through its network alias, the orderer host in the config block, see
	// fabricx_network.OrdererHost.
	logger.Infof("Run fabric-x mock orderer container on port %v", orderingServicePort)
	// The orderer reads only its own settings from the first committer's configuration, and
	// its env from the topology's orderer env rather than from the committer's.
	ordererCfg := e.committers[0].containerConfig()
	ordererCfg.EnvVarOverrides = e.ordererEnvVars
	e.runContainer(e.ordererContainerName(), []string{fabricx_network.OrdererHost}, ordererCfg, []string{"orderer"},
		[]int{orderingServicePort},
		[]mount.Mount{crypto, block, {Type: mount.TypeBind, Source: mockOrdererConfigPath, Target: "/root/config/mock-orderer.yaml", ReadOnly: true}},
	)

	// Each committer runs in a container of its own, with its own database.
	for _, c := range e.committers {
		sidecarPort, queryServicePort := c.ports()
		logger.Infof("Run fabric-x committer container [%s] on ports: sidecar=%v query=%v", c.containerName(), sidecarPort, queryServicePort)
		e.runContainer(c.containerName(), nil, c.containerConfig(), []string{"db", "committer"},
			[]int{sidecarPort, queryServicePort},
			[]mount.Mount{crypto, block},
		)
	}
}

// containerConfig returns the configuration of the committer's container.
func (c *committer) containerConfig() containerConfig {
	e := c.ext
	sidecarPort, queryServicePort := c.ports()
	return containerConfig{
		ChannelName:           e.channel.Name,
		SidecarMSPDir:         containerSidecarMSPDir(e.network, c.sidecarPeer()),
		SidecarMSPID:          fmt.Sprintf("%sMSP", c.cfg.SidecarOrg),
		SidecarServerEndpoint: net.JoinHostPort("", strconv.Itoa(sidecarPort)),
		QueryServerEndpoint:   net.JoinHostPort("", strconv.Itoa(queryServicePort)),
		OrdererServerEndpoint: net.JoinHostPort("", strconv.Itoa(e.orderingPort())),
		TLSEnabled:            e.network.TLSEnabled,
		CertsBundle:           path.Join("/root/artifacts/crypto", "ca-certs.pem"),
		SidecarTLSDir:         containerSidecarTLSDir(e.network, c.sidecarPeer()),
		OrdererTLSDir:         containerOrdererTLSDir(e.network, e.network.Orderers[0]),
		Image:                 c.cfg.Image,
		EnvVarOverrides:       c.cfg.EnvVars,
	}
}

// runContainer creates and starts a container of the committer test image named name, running
// the image's run script for ops, publishing ports on the host, with mounts, and waits until
// every port in ports accepts connections on the host. It joins the network's Docker network
// under aliases, and on Linux maps host.docker.internal to the host gateway, as Docker Desktop
// does itself.
func (e *Extension) runContainer(name string, aliases []string, cfg containerConfig, ops []string, ports []int, mounts []mount.Mount) {
	var extraHosts []string
	if runtime.GOOS == "linux" {
		extraHosts = append(extraHosts, "host.docker.internal:host-gateway")
	}

	cli, err := dcli.New(dcli.FromEnv)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	ctx := context.TODO()
	resp, err := cli.ContainerCreate(
		ctx,
		dcli.ContainerCreateOptions{
			Name: name,
			Config: &container.Config{
				Image:        containerImage(cfg),
				Tty:          true,
				AttachStdout: true,
				AttachStderr: true,
				ExposedPorts: docker.PortSet(ports...),
				Env:          containerEnvVars(cfg),
				Cmd:          containerCmd(cfg, ops...),
			},
			HostConfig: &container.HostConfig{
				ExtraHosts:   extraHosts,
				Mounts:       mounts,
				PortBindings: docker.PortBindings(ports...),
			},
			NetworkingConfig: &network.NetworkingConfig{
				EndpointsConfig: map[string]*network.EndpointSettings{
					e.network.NetworkID: {Aliases: aliases},
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
	e.waitUntilReady(e.forwardLogs(name, logs, func() { _ = logs.Close() }), ports...)
}

// forwardLogs forwards what the container named containerName writes to r to the
// logger until r ends, then calls closeReader. The returned context is canceled once
// r ends, which happens when the processes writing to it exit.
func (*Extension) forwardLogs(containerName string, r io.Reader, closeReader func()) context.Context {
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

// waitUntilReady waits until every port in ports accepts connections on the host, over TLS
// when the network uses it, presenting the first committer's admin credentials, which every
// service accepts since each trusts every organization's CA. The test fails if they do not
// within 90 seconds or ctx ends first.
func (e *Extension) waitUntilReady(ctx context.Context, ports ...int) {
	sidecarPeer := e.committers[0].sidecarPeer()

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
	for _, p := range ports {
		g.Go(func() error {
			addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(p))
			return fabric.WaitUntilReadyWithTLS(ctx, addr, tlsConfig)
		})
	}
	gomega.Expect(g.Wait()).NotTo(gomega.HaveOccurred())
}
