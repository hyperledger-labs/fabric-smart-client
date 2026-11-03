/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// preambleStream is a stream to peer that carries pre and blocks reads until closed.
type preambleStream struct {
	host.P2PStream
	peer, hash string
	pre        *preamble.Preamble
	ctx        context.Context
	cancel     context.CancelFunc
}

func newPreambleStream(peer, hash string, pre *preamble.Preamble) *preambleStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &preambleStream{peer: peer, hash: hash, pre: pre, ctx: ctx, cancel: cancel}
}

func (s *preambleStream) RemotePeerID() host.PeerID    { return s.peer }
func (*preambleStream) RemotePeerAddress() string      { return "addr" }
func (s *preambleStream) Hash() host.StreamHash        { return s.hash }
func (s *preambleStream) Context() context.Context     { return s.ctx }
func (s *preambleStream) Preamble() *preamble.Preamble { return s.pre }
func (*preambleStream) Write(p []byte) (int, error)    { return len(p), nil }
func (s *preambleStream) Close() error                 { s.cancel(); return nil }
func (s *preambleStream) Read([]byte) (int, error)     { <-s.ctx.Done(); return 0, io.EOF }

func TestPeerPreamble(t *testing.T) {
	t.Parallel()
	p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)

	_, ok := p.PeerPreamble("peer")
	require.False(t, ok, "no stream")

	old := newPreambleStream("peer", "h1", nil)
	p.handleStream(old)
	_, ok = p.PeerPreamble("peer")
	require.False(t, ok, "a peer without a preamble")

	v1 := &preamble.Preamble{FscVersion: "v1"}
	first := newPreambleStream("peer", "h2", v1)
	p.handleStream(first)
	v2 := &preamble.Preamble{FscVersion: "v2"}
	second := newPreambleStream("peer", "h3", v2)
	p.handleStream(second)
	got, ok := p.PeerPreamble("peer")
	require.True(t, ok)
	require.Same(t, v2, got, "the newest live stream wins")

	// Once a stream dies, its preamble is no longer reported.
	require.NoError(t, second.Close())
	require.Eventually(t, func() bool {
		got, ok := p.PeerPreamble("peer")
		return ok && got == v1
	}, 5*time.Second, tick)
	_, ok = p.PeerPreamble("other")
	require.False(t, ok)
}
