/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

//go:generate counterfeiter -o mock/libp2p_config.go -fake-name LibP2PConfig . libp2pConfig

func TestMain(m *testing.M) {
	spec := os.Getenv("FABRIC_LOGGING_SPEC")
	if len(spec) == 0 {
		spec = os.Getenv("FSC_LOGSPEC")
	}
	if len(spec) == 0 {
		spec = "error"
	}
	if err := logging.Init(logging.Config{
		LogSpec: spec,
	}); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestP2PLayerTestRound(t *testing.T) {
	t.Parallel()
	node1, node2 := setupTwoNodes(t)
	comm.P2PLayerTestRound(t, node1, node2)
}

func TestSessionsTestRound(t *testing.T) {
	t.Parallel()
	node1, node2 := setupTwoNodes(t)
	comm.SessionsTestRound(t, node1, node2)
}

func TestSessionsForMPCTestRound(t *testing.T) {
	t.Parallel()
	node1, node2 := setupTwoNodes(t)
	comm.SessionsForMPCTestRound(t, node1, node2)
}

// The 100 sessions start at once on nodes that are not connected yet.
func TestSessionsMultipleMessagesTestRound(t *testing.T) {
	t.Parallel()
	node1, node2 := setupTwoNodes(t)
	comm.SessionsMultipleMessagesTestRound(t, node1, node2)
}

func TestSessionsTwoNodesTestRound(t *testing.T) {
	t.Parallel()
	nodes := setupNodes(t, 3)
	comm.SessionsNodesTestRound(t, nodes[0], nodes[1:], 2)
}

// A node starts while its peer is not listening yet, and reaches the peer once it is.
func TestStartWhilePeerDown(t *testing.T) {
	t.Parallel()
	addrs := freeLibP2PAddresses(t, 2)
	aSK, aID := generateKey(t)
	bSK, bID := generateKey(t)
	a := newTestNode(t, aSK, aID, addrs[0])
	b := newTestNode(t, bSK, bID, addrs[1])
	comm.SessionsTestRound(t, a, b)
}

// Two nodes that are not connected send to each other at the same moment.
func TestSimultaneousDial(t *testing.T) {
	t.Parallel()
	a, b := setupTwoNodes(t)
	ctx := t.Context()
	a.Start(ctx)
	b.Start(ctx)
	defer a.Stop()
	defer b.Stop()

	sa, err := a.NewResponderSession("s", "c", b.Address, []byte(b.ID), nil, nil)
	require.NoError(t, err)
	sb, err := b.NewResponderSession("s", "c", a.Address, []byte(a.ID), nil, nil)
	require.NoError(t, err)
	errs := make(chan error, 2)
	go func() { errs <- sa.Send(ctx, []byte("a")) }()
	go func() { errs <- sb.Send(ctx, []byte("b")) }()
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
}

// TestReplyAfterConnectionClosed checks that a responder can still reply once the connection is
// gone. The reply goes to the source address of the incoming stream, an ephemeral port, so it
// reaches the peer only through the listen address the identify protocol recorded. Only the side
// that opens a stream waits for identify; z receives the stream, so the test waits until z has
// recorded a's listen address.
func TestReplyAfterConnectionClosed(t *testing.T) {
	t.Parallel()
	addrs := freeLibP2PAddresses(t, 2)
	aSK, aID := generateKey(t)
	zSK, zID := generateKey(t)
	a := newTestNode(t, aSK, aID, addrs[0])
	zCfg := &mock.LibP2PConfig{}
	zCfg.ListenAddressReturns(addrs[1])
	zHost, err := newLibP2PHost(zCfg, zSK, newMetrics(&disabled.Provider{}))
	require.NoError(t, err)
	zNode, err := comm.NewNode(t.Context(), zHost, &disabled.Provider{})
	require.NoError(t, err)
	z := &comm.HostNode{P2PNode: zNode, ID: zID, Address: addrs[1]}
	ctx := t.Context()
	a.Start(ctx)
	z.Start(ctx)
	defer a.Stop()
	defer z.Stop()

	master, err := z.MasterSession()
	require.NoError(t, err)
	s, err := a.NewSession("", "", z.Address, []byte(z.ID))
	require.NoError(t, err)
	require.NoError(t, s.Send(ctx, []byte("ping")))
	msg := <-master.Receive()

	aPeer, err := peer.Decode(aID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return slices.ContainsFunc(zHost.Peerstore().Addrs(aPeer), func(ma multiaddr.Multiaddr) bool {
			return ma.String() == a.Address
		})
	}, 10*time.Second, 10*time.Millisecond, "z never learned a's listen address")
	require.NoError(t, zHost.Network().ClosePeer(aPeer))

	r, err := z.NewResponderSession(msg.SessionID, msg.ContextID, msg.FromEndpoint, msg.FromPKID, nil, nil)
	require.NoError(t, err)
	require.NoError(t, r.Send(ctx, []byte("pong")))
	select {
	case m := <-s.Receive():
		require.Equal(t, []byte("pong"), m.Payload)
	case <-time.After(10 * time.Second):
		t.Fatal("no reply")
	}
}

func TestSessionChurn_NoLostReplies(t *testing.T) {
	t.Parallel()
	a, z := setupTwoNodes(t)
	ctx := context.Background()
	a.Start(ctx)
	z.Start(ctx)
	defer a.Stop()
	defer z.Stop()

	master, err := z.MasterSession()
	require.NoError(t, err)
	go func() {
		for msg := range master.Receive() {
			s, err := z.NewResponderSession(msg.SessionID, msg.ContextID, "", msg.FromPKID, nil, nil) //nolint:contextcheck // NewResponderSession takes no context, as in comm.SessionsMultipleMessagesTestRound
			if err != nil {
				continue
			}
			go func() {
				_ = s.Send(ctx, msg.Payload)
				for m := range s.Receive() {
					if string(m.Payload) == "bye" {
						s.Close()
						return
					}
				}
			}()
		}
	}()

	for i := range 200 {
		s, err := a.NewSession("", "", z.Address, []byte(z.ID))
		require.NoError(t, err)
		require.NoError(t, s.Send(ctx, []byte("ping")))
		select {
		case <-s.Receive():
		case <-time.After(5 * time.Second):
			t.Fatalf("iteration %d: no reply", i)
		}
		_ = s.Send(ctx, []byte("bye"))
		s.Close()
	}
}

// TestSessionClosedWhenPeerDies checks that a session whose remote node stops
// closes its Receive channel once no replacement stream arrives within
// sessionStreamGrace, so a caller ranging over it does not block forever on a
// dead peer.
func TestSessionClosedWhenPeerDies(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	alice, bob := setupTwoNodes(t)

	alice.Start(ctx)
	bob.Start(ctx)

	sess, err := alice.NewSession("", "", bob.Address, []byte(bob.ID))
	require.NoError(t, err)
	require.NoError(t, sess.Send(ctx, []byte("hi! bob")))

	msess, err := bob.MasterSession()
	require.NoError(t, err)
	select {
	case msg := <-msess.Receive():
		require.Equal(t, []byte("hi! bob"), msg.Payload)
	case <-time.After(5 * time.Second):
		t.Fatal("bob did not receive alice's first message")
	}

	bob.Stop()

	// sessionStreamGrace (5s) plus 10s of slack. Created once, so messages
	// drained before the close do not restart the timer.
	deadline := time.After(15 * time.Second)
	for {
		select {
		case _, ok := <-sess.Receive():
			if !ok {
				return // channel closed: the session noticed bob is gone
			}
		case <-deadline:
			t.Fatal("session to a stopped peer did not close within 15s")
		}
	}
}

func generateKey(t *testing.T) (crypto.PrivKey, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privKey, pubKey, err := crypto.ECDSAKeyPairFromKey(priv)
	require.NoError(t, err)
	ID, err := peer.IDFromPublicKey(pubKey)
	require.NoError(t, err)
	return privKey, ID.String()
}

func freeLibP2PAddresses(t *testing.T, n int) []string {
	t.Helper()
	listeners := make([]net.Listener, n)
	addresses := make([]string, n)
	for i := range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		listeners[i] = l
		addresses[i] = fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", l.Addr().(*net.TCPAddr).Port)
	}
	for _, l := range listeners {
		require.NoError(t, l.Close())
	}
	return addresses
}

// newTestNode starts a comm node on a libp2p host with key sk listening at addr.
func newTestNode(t *testing.T, sk crypto.PrivKey, id, addr string) *comm.HostNode {
	t.Helper()
	cfg := &mock.LibP2PConfig{}
	cfg.ListenAddressReturns(addr)
	h, err := newLibP2PHost(cfg, sk, newMetrics(&disabled.Provider{}))
	require.NoError(t, err)
	n, err := comm.NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	return &comm.HostNode{P2PNode: n, ID: id, Address: addr}
}

func setupNodes(t *testing.T, n int) []*comm.HostNode {
	t.Helper()
	addrs := freeLibP2PAddresses(t, n)
	nodes := make([]*comm.HostNode, n)
	for i, addr := range addrs {
		sk, id := generateKey(t)
		nodes[i] = newTestNode(t, sk, id, addr)
	}
	return nodes
}

// setupTwoNodes returns two nodes that are not connected yet.
func setupTwoNodes(t *testing.T) (node1, node2 *comm.HostNode) {
	t.Helper()
	nodes := setupNodes(t, 2)
	return nodes[0], nodes[1]
}
