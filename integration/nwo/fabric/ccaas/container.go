/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ccaas

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	dcli "github.com/moby/moby/client"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

var logger = logging.MustGetLogger()

// ContainerSpec describes one chaincode server container.
type ContainerSpec struct {
	Name          string
	Image         string
	NetworkID     string
	Port          uint16
	CCID          string
	ServerAddress string // address the server binds inside the container, e.g. 0.0.0.0:<port>
	MSPID         string // CORE_PEER_LOCALMSPID for the org this server endorses for
	Env           []string
	Mounts        []Mount
}

// ContainerManager tracks started chaincode containers so they can be removed.
type ContainerManager struct {
	cli dcli.APIClient
	ids []string
}

// NewContainerManager creates a docker-backed ContainerManager using the
// environment-derived docker client configuration.
func NewContainerManager() (*ContainerManager, error) {
	cli, err := dcli.New(dcli.FromEnv)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create docker client")
	}
	return &ContainerManager{cli: cli}, nil
}

// Start creates and starts the container, then blocks until its published port
// accepts a TCP connection or a timeout elapses.
func (m *ContainerManager) Start(spec ContainerSpec) error {
	ctx := context.Background()
	port := int(spec.Port)

	env := containerEnv(spec)

	mounts := make([]mount.Mount, 0, len(spec.Mounts))
	for _, mt := range spec.Mounts {
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: mt.Source, Target: mt.Target, ReadOnly: true})
	}

	resp, err := m.cli.ContainerCreate(ctx, dcli.ContainerCreateOptions{
		Name: spec.Name,
		Config: &container.Config{
			Image:        spec.Image,
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
			ExposedPorts: docker.PortSet(port),
			Env:          env,
		},
		HostConfig: &container.HostConfig{
			Mounts:       mounts,
			PortBindings: docker.PortBindings(port),
		},
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{spec.NetworkID: {}},
		},
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create chaincode container %s (image %s)", spec.Name, spec.Image)
	}
	m.ids = append(m.ids, resp.ID)

	if _, err := m.cli.ContainerStart(ctx, resp.ID, dcli.ContainerStartOptions{}); err != nil {
		return errors.Wrapf(err, "failed to start chaincode container %s on port %d", spec.Name, port)
	}
	if err := docker.StartLogs(m.cli, resp.ID, "ccaas."+spec.Name); err != nil {
		logger.Warnf("failed to pipe logs for %s: %v", spec.Name, err)
	}

	return waitForPort(fmt.Sprintf("127.0.0.1:%d", port), 30*time.Second)
}

// containerEnv builds the chaincode server's environment. CORE_PEER_LOCALMSPID
// is what shim.GetMSPID reads: chaincode that checks the endorsing peer's org
// needs it, and the peer only injects it for chaincode it launches itself.
// Caller-supplied vars come last so a chaincode extension can override.
func containerEnv(spec ContainerSpec) []string {
	return append([]string{
		"CHAINCODE_ID=" + spec.CCID,
		"CHAINCODE_SERVER_ADDRESS=" + spec.ServerAddress,
		"CHAINCODE_TLS=false",
		"CORE_PEER_LOCALMSPID=" + spec.MSPID,
	}, spec.Env...)
}

// StopAll force-removes all tracked containers, best-effort.
func (m *ContainerManager) StopAll() error {
	ctx := context.Background()
	var errs []error
	for _, id := range m.ids {
		if _, err := m.cli.ContainerRemove(ctx, id, dcli.ContainerRemoveOptions{Force: true}); err != nil {
			errs = append(errs, err)
		}
	}
	m.ids = nil
	return errors.Join(errs...)
}

func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.Errorf("chaincode server at %s not reachable within %s", addr, timeout)
}
