/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/tlsgen"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/ws"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// node is a started comm node together with the address and ID a peer dials it by.
type node struct {
	*comm.P2PNode
	ID      host.PeerID
	Address host.PeerIPAddress
}

// transport starts n nodes, each able to open sessions to every other, over one P2P transport.
type transport struct {
	name  string
	setup func(b *testing.B, n int) []*node
}

// pair starts two nodes over t.
func (t transport) pair(b *testing.B) (a, z *node) {
	b.Helper()
	nodes := t.setup(b, 2)
	return nodes[0], nodes[1]
}

var transports = []transport{
	{name: "websocket", setup: setupWebsocket},
	{name: "libp2p", setup: setupLibP2P},
}

// nodeKeys are the PEM files of a node's ECDSA P-256 key and its CA-issued certificate.
type nodeKeys struct {
	keyPath, certPath string
	pub               *ecdsa.PublicKey
}

// generateKeys writes a CA and n node key pairs, issued by that CA, into a temporary directory.
// libp2p uses only the keys; websocket uses the keys, the certificates and the CA for mutual TLS.
func generateKeys(b *testing.B, n int) (caPath string, nodes []nodeKeys) {
	b.Helper()
	dir := b.TempDir()

	ca, err := tlsgen.NewCA()
	require.NoError(b, err)
	caPath = filepath.Join(dir, "ca.pem")
	require.NoError(b, os.WriteFile(caPath, ca.CertBytes(), 0o600))

	for i := range n {
		kp, err := ca.NewServerCertKeyPair("127.0.0.1")
		require.NoError(b, err)
		k := nodeKeys{
			keyPath:  filepath.Join(dir, fmt.Sprintf("node%d-key.pem", i)),
			certPath: filepath.Join(dir, fmt.Sprintf("node%d-cert.pem", i)),
			pub:      kp.Public().(*ecdsa.PublicKey),
		}
		require.NoError(b, os.WriteFile(k.keyPath, kp.Key, 0o600))
		require.NoError(b, os.WriteFile(k.certPath, kp.Cert, 0o600))
		nodes = append(nodes, k)
	}
	return caPath, nodes
}

// freePorts returns n distinct TCP ports on 127.0.0.1 that were free when the call returned.
func freePorts(b *testing.B, n int) []int {
	b.Helper()
	ports := make([]int, n)
	listeners := make([]net.Listener, n)
	for i := range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(b, err)
		listeners[i] = l
		ports[i] = l.Addr().(*net.TCPAddr).Port
	}
	for _, l := range listeners {
		require.NoError(b, l.Close())
	}
	return ports
}

func startNode(b *testing.B, h host.P2PHost, id host.PeerID, address host.PeerIPAddress) *node {
	b.Helper()
	p, err := comm.NewNode(context.Background(), h, &disabled.Provider{})
	require.NoError(b, err)
	p.Start(context.Background())
	b.Cleanup(p.Stop)
	return &node{P2PNode: p, ID: id, Address: address}
}

// setupWebsocket starts n websocket nodes that authenticate each other with mutual TLS and find
// each other through a static ID router listing all of them.
func setupWebsocket(b *testing.B, n int) []*node {
	b.Helper()
	caPath, keys := generateKeys(b, n)
	ports := freePorts(b, n)

	ids := make([]host.PeerID, n)
	addresses := make([]host.PeerIPAddress, n)
	router := routing.StaticIDRouter{}
	for i, k := range keys {
		id, err := websocket.PKIDSynthesizer{}.PublicKeyID(k.pub)
		require.NoError(b, err)
		ids[i] = string(id)
		addresses[i] = fmt.Sprintf("127.0.0.1:%d", ports[i])
		router[ids[i]] = []host.PeerIPAddress{addresses[i]}
	}

	nodes := make([]*node, n)
	for i, k := range keys {
		cfg, err := websocket.NewConfigFromProperties(addresses[i], k.keyPath, k.certPath, []string{caPath}, []string{caPath}, true, 100, nil)
		require.NoError(b, err)
		discovery := routing.NewServiceDiscovery(&router, routing.RoundRobin[host.PeerIPAddress]())
		h, err := websocket.NewHost(ids[i], discovery, ws.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0), cfg, noExtraCAs{})
		require.NoError(b, err)
		nodes[i] = startNode(b, h, ids[i], addresses[i])
	}
	return nodes
}

type noExtraCAs struct{}

func (noExtraCAs) ExtraCAs() [][]byte { return nil }

// setupLibP2P starts a libp2p bootstrap node and n-1 nodes that join through it. Sessions are
// opened by address, so any node can reach any other without waiting for DHT discovery.
func setupLibP2P(b *testing.B, n int) []*node {
	b.Helper()
	_, keys := generateKeys(b, n)
	ports := freePorts(b, n)

	bootstrapTCP := fmt.Sprintf("127.0.0.1:%d", ports[0])
	nodes := make([]*node, n)
	for i, k := range keys {
		cfg := &libp2pConfig{keyPath: k.keyPath, listen: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", ports[i])}
		var resolver *bootstrapResolver
		if i > 0 {
			cfg.bootstrap = bootstrapTCP
			resolver = &bootstrapResolver{address: bootstrapTCP, id: nodes[0].ID}
		}
		h, err := libp2p.NewHostGeneratorProvider(cfg, &disabled.Provider{}, resolver).GetNewHost()
		require.NoError(b, err)
		nodes[i] = startNode(b, h, h.PeerID(), cfg.listen)
	}
	return nodes
}

// libp2pConfig is the configuration of a libp2p host with the connection manager defaults of
// libp2p.NewConfig.
type libp2pConfig struct {
	keyPath, listen, bootstrap string
}

func (c *libp2pConfig) PrivateKeyPath() string                     { return c.keyPath }
func (c *libp2pConfig) Bootstrap() bool                            { return c.bootstrap == "" }
func (c *libp2pConfig) ListenAddress() host.PeerIPAddress          { return c.listen }
func (c *libp2pConfig) BootstrapListenAddress() host.PeerIPAddress { return c.bootstrap }
func (*libp2pConfig) ConnManagerLowWater() int                     { return 100 }
func (*libp2pConfig) ConnManagerHighWater() int                    { return 400 }
func (*libp2pConfig) ConnManagerGracePeriod() time.Duration        { return time.Minute }

// bootstrapResolver resolves any party to the bootstrap node, which is all a joining libp2p
// host asks the endpoint service for.
type bootstrapResolver struct {
	address string
	id      host.PeerID
}

func (*bootstrapResolver) GetIdentity(string, []byte) (view.Identity, error) {
	return view.Identity("bootstrap"), nil
}

func (r *bootstrapResolver) Resolve(context.Context, view.Identity) (view.Identity, map[endpoint.PortName]string, []byte, error) {
	return nil, map[endpoint.PortName]string{endpoint.P2PPort: r.address}, []byte(r.id), nil
}
