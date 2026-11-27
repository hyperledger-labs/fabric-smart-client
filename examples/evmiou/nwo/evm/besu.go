/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Adapted from Panurus (x/token/services/network/evm) at commit f144b2b7f.
package evm

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/moby/moby/api/types/container"
	dcli "github.com/moby/moby/client"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/docker"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

var logger = logging.MustGetLogger()

// DefaultBesuImage is the image StartBesu boots.
const DefaultBesuImage = "hyperledger/besu:24.3.0"

// DefaultChainID is the chain ID Besu's development network runs with.
const DefaultChainID int64 = 1337

// besuReadyTimeout bounds how long StartBesu waits for the node to answer eth_chainId.
const besuReadyTimeout = 2 * time.Minute

// BesuConfig describes the Besu development node to boot.
type BesuConfig struct {
	// Name is the container name.
	Name string
	// Port is the host port the JSON-RPC endpoint is published on; Besu listens on it too, so no
	// host/container port translation is needed.
	Port uint16
}

// Besu is a running Besu development node.
type Besu struct {
	containerID string
	endpoint    string
}

// Endpoint returns the node's JSON-RPC URL as reachable from the host.
func (b *Besu) Endpoint() string { return b.endpoint }

// StartBesu boots a Besu node in development mode (instant mining, pre-funded dev accounts) and
// waits until it answers eth_chainId.
func StartBesu(ctx context.Context, cfg BesuConfig) (*Besu, error) {
	if cfg.Port == 0 {
		return nil, errors.New("evm nwo: no host port assigned for besu")
	}
	image := DefaultBesuImage

	d, err := docker.GetInstance()
	if err != nil {
		return nil, errors.Wrap(err, "evm nwo: docker is not available")
	}
	if err := d.CheckImagesExist(image); err != nil { //nolint:contextcheck // CheckImagesExist takes no context; shared docker helper, every caller is in the same position
		return nil, errors.Wrapf(err, "evm nwo: the besu image [%s] is missing; pull it first", image)
	}

	resp, err := d.Client.ContainerCreate(ctx, dcli.ContainerCreateOptions{
		Name: cfg.Name,
		Config: &container.Config{
			Image:        image,
			Tty:          true,
			AttachStdout: true,
			AttachStderr: true,
			ExposedPorts: docker.PortSet(int(cfg.Port)),
			Cmd:          besuArgs(cfg.Port),
		},
		HostConfig: &container.HostConfig{
			PortBindings: docker.PortBindings(int(cfg.Port)),
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "evm nwo: failed to create the besu container")
	}

	if _, err := d.Client.ContainerStart(ctx, resp.ID, dcli.ContainerStartOptions{}); err != nil {
		return nil, errors.Wrap(err, "evm nwo: failed to start the besu container")
	}

	if err := docker.StartLogs(d.Client, resp.ID, "besu."+resp.ID[:8]); err != nil { //nolint:contextcheck // StartLogs takes no context; shared docker helper, every caller is in the same position
		// Logs are diagnostics; losing them must not fail the network.
		logger.Warnf("evm nwo: could not stream besu logs: %s", err)
	}

	node := &Besu{containerID: resp.ID, endpoint: "http://127.0.0.1:" + strconv.Itoa(int(cfg.Port))}
	if err := node.waitReady(ctx); err != nil {
		_, _ = d.Client.ContainerRemove(ctx, node.containerID, dcli.ContainerRemoveOptions{Force: true})

		return nil, err
	}
	logger.Infof("evm nwo: besu is up at %s", node.endpoint)

	return node, nil
}

// Stop removes the container. It is safe to call on a nil Besu.
func (b *Besu) Stop(ctx context.Context) error {
	if b == nil || b.containerID == "" {
		return nil
	}
	d, err := docker.GetInstance()
	if err != nil {
		return errors.Wrap(err, "evm nwo: docker is not available")
	}
	if _, err := d.Client.ContainerRemove(ctx, b.containerID, dcli.ContainerRemoveOptions{Force: true}); err != nil {
		return errors.Wrap(err, "evm nwo: failed to remove the besu container")
	}

	return nil
}

// besuMinerCoinbase is the beneficiary address Besu requires when mining is enabled. Development
// mode pre-funds its own fixed set of accounts independently of this value, so any valid address
// does; the dev network's genesis, not the coinbase, decides who starts funded.
const besuMinerCoinbase = "0xf17f52151EbEF6C7334FAD080c5704D77216b732"

// besuArgs builds the development-mode command line: instant mining, JSON-RPC open on port, and
// chain ID 1337.
func besuArgs(port uint16) []string {
	return []string{
		"--network=dev",
		"--miner-enabled",
		"--miner-coinbase=" + besuMinerCoinbase,
		"--min-gas-price=0",
		"--rpc-http-enabled",
		"--rpc-http-host=0.0.0.0",
		fmt.Sprintf("--rpc-http-port=%d", port),
		"--rpc-http-api=ETH,NET,WEB3,TXPOOL",
		"--rpc-http-cors-origins=all",
		"--host-allowlist=*",
		fmt.Sprintf("--network-id=%d", DefaultChainID),
	}
}

// waitReady polls eth_chainId until the node answers, so callers get a node that is actually
// usable rather than one whose container merely started.
func (b *Besu) waitReady(ctx context.Context) error {
	rpc, err := client.NewJSONRPCClient(b.endpoint, &http.Client{Timeout: 2 * time.Second})
	if err != nil {
		return err
	}

	deadline := time.Now().Add(besuReadyTimeout)
	for {
		if err := rpc.Ping(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.Errorf("evm nwo: besu did not become ready at %s within %s", b.endpoint, besuReadyTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
