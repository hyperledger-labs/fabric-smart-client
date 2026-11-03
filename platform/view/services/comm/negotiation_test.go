/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"bytes"
	"context"
	stdio "io"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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
func (s *preambleStream) Read([]byte) (int, error)     { <-s.ctx.Done(); return 0, stdio.EOF }

// preambleHost opens streams that carry pre, as the peer's preamble, and record what is written.
type preambleHost struct {
	mockHost
	pre     *preamble.Preamble
	mu      sync.Mutex
	written bytes.Buffer
	opened  int
}

func (h *preambleHost) NewStream(_ context.Context, info host.StreamInfo) (host.P2PStream, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.opened++
	return &recordingStream{preambleStream: newPreambleStream(info.RemotePeerID, "hash", h.pre), h: h}, nil
}

func (*preambleHost) StreamHash(host.StreamInfo) host.StreamHash { return "hash" }

// packets decodes what was written to the host's streams.
func (h *preambleHost) packets(t *testing.T) []*ViewPacket {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	r := io.NewVarintProtoReader(bytes.NewReader(h.written.Bytes()), 1024, 1<<20)
	var out []*ViewPacket
	for {
		p := &ViewPacket{}
		if err := r.ReadMsg(p); err != nil {
			return out
		}
		out = append(out, p)
	}
}

type recordingStream struct {
	*preambleStream
	h *preambleHost
}

func (s *recordingStream) Write(p []byte) (int, error) {
	s.h.mu.Lock()
	defer s.h.mu.Unlock()
	return s.h.written.Write(p)
}

func newInitiatorSession(t *testing.T, peer *preamble.Preamble) (*NetworkStreamSession, *preambleHost) {
	t.Helper()
	h := &preambleHost{pre: peer}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)
	s, err := p.NewSession("app/Initiator", "ctx", "addr", []byte("peer"))
	require.NoError(t, err)
	return s.(*NetworkStreamSession), h
}

func peerWith(ranges ...*preamble.ComponentRange) *preamble.Preamble {
	return &preamble.Preamble{FramingEpoch: preamble.FramingEpoch, Ranges: ranges}
}

// TestInitiatorNegotiates runs against the default components: view, fabric and fabricx at
// {1, 1}, so every version a session uses is 1 and only the peer's range decides overlap.
func TestInitiatorNegotiates(t *testing.T) {
	t.Parallel()

	t.Run("states the same versions and protocol on every message", func(t *testing.T) {
		t.Parallel()
		s, h := newInitiatorSession(t, peerWith(&preamble.ComponentRange{Component: "view", Min: 1, Max: 3}))
		s.SetProtocol("pingpong/ping")
		require.NoError(t, s.Send(t.Context(), []byte("one")))
		require.NoError(t, s.Send(t.Context(), []byte("two")))

		packets := h.packets(t)
		require.Len(t, packets, 2)
		for _, p := range packets {
			require.Equal(t, "pingpong/ping", p.GetProtocol())
			require.Equal(t, map[string]uint32{"view": 1, "fabric": 1, "fabricx": 1}, statedVersions(t, p))
		}
		require.Equal(t, []byte("one"), packets[0].GetPayload(), "payload bytes are unchanged")
	})

	t.Run("Version before the first Send opens the stream and pins the result", func(t *testing.T) {
		t.Parallel()
		s, h := newInitiatorSession(t, nil)
		v, ok, err := s.Version(t.Context(), "view")
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, uint32(1), v, "a peer without a preamble speaks version 1")
		require.Equal(t, 1, h.opened)
		require.NoError(t, s.Send(t.Context(), []byte("x")))
		require.Equal(t, 1, h.opened, "Send reuses the stream negotiate opened")

		s.Close()
		_, _, err = s.Version(t.Context(), "fabric")
		require.ErrorIs(t, err, ErrSessionClosed)
	})

	t.Run("no overlap fails with both ranges and is stated as 0", func(t *testing.T) {
		t.Parallel()
		s, h := newInitiatorSession(t, peerWith(&preamble.ComponentRange{Component: "fabric", Min: 2, Max: 3}))
		_, _, err := s.Version(t.Context(), "fabric")
		require.ErrorIs(t, err, view.ErrNoCommonVersion)
		require.EqualError(t, err, "component [fabric]: local [1, 1], peer [2, 3]: no common version")
		v, err := view.SessionVersion(t.Context(), s, "view")
		require.NoError(t, err, "other components still negotiate")
		require.Equal(t, uint32(1), v)

		require.NoError(t, s.Send(t.Context(), nil))
		require.Equal(t, uint32(0), statedVersions(t, h.packets(t)[0])["fabric"])
	})

	t.Run("a component this node does not have", func(t *testing.T) {
		t.Parallel()
		s, _ := newInitiatorSession(t, nil)
		_, ok, err := s.Version(t.Context(), "unknown")
		require.NoError(t, err)
		require.False(t, ok)
	})
}

// TestInitiatorChoosesHighest changes the process-wide components, so it runs alone and restores
// them before the parallel tests of the package run.
func TestInitiatorChoosesHighest(t *testing.T) { //nolint:paralleltest
	require.NoError(t, preamble.SetComponents(append(slices.Clone(preamble.FSCComponents), preamble.Component{Name: "iou", Range: preamble.Range{Min: 1, Max: 3}})))
	t.Cleanup(func() { require.NoError(t, preamble.SetComponents(preamble.FSCComponents)) })

	peer := peerWith(&preamble.ComponentRange{Component: "iou", Min: 2, Max: 5})
	s, h := newInitiatorSession(t, peer)
	v, err := view.SessionVersion(t.Context(), s, "iou")
	require.NoError(t, err)
	require.Equal(t, uint32(3), v, "the highest version in both ranges")
	require.NoError(t, s.Send(t.Context(), nil))
	require.Equal(t, uint32(3), statedVersions(t, h.packets(t)[0])["iou"])

	// Both nodes open a session by ID with NewSessionWithID; each negotiates on its own and
	// picks the same version from the other's preamble.
	h = &preambleHost{pre: peer}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)
	byID, err := p.NewSessionWithID("agreed-id", "ctx", "addr", []byte("peer"))
	require.NoError(t, err)
	v, err = view.SessionVersion(t.Context(), byID, "iou")
	require.NoError(t, err)
	require.Equal(t, uint32(3), v, "a session opened by ID negotiates")
	require.NoError(t, byID.Send(t.Context(), nil))
	require.Equal(t, uint32(3), statedVersions(t, h.packets(t)[0])["iou"])
}

// statedVersions decodes the versions p states.
func statedVersions(t *testing.T, p *ViewPacket) map[string]uint32 {
	t.Helper()
	m, err := preamble.DecodeVersions(p.GetVersions())
	require.NoError(t, err)
	return m
}

func TestResponderVersions(t *testing.T) {
	t.Parallel()
	p, err := NewNode(t.Context(), &mockHost{}, &disabled.Provider{})
	require.NoError(t, err)
	t.Cleanup(p.Stop)

	newResponder := func(id string, versions map[string]uint32) view.Session {
		var raw []byte
		if versions != nil {
			raw, err = preamble.EncodeVersions(versions)
			require.NoError(t, err)
		}
		msg := &view.Message{SessionID: id, Versions: raw, Ctx: t.Context()}
		s, err := p.NewResponderSession(id, "ctx", "addr", []byte("peer"), view.Identity("caller"), msg)
		require.NoError(t, err)
		return s
	}

	s := newResponder("stated", map[string]uint32{"view": 1})
	v, err := view.SessionVersion(t.Context(), s, "view")
	require.NoError(t, err)
	require.Equal(t, uint32(1), v)

	s = newResponder("old-initiator", nil)
	v, err = view.SessionVersion(t.Context(), s, "fabric")
	require.NoError(t, err)
	require.Equal(t, uint32(1), v, "an initiator that states nothing runs an older release, at version 1")

	s = newResponder("outside", map[string]uint32{"view": 2})
	_, err = view.SessionVersion(t.Context(), s, "view")
	require.ErrorIs(t, err, view.ErrNoCommonVersion)

	s = newResponder("no-overlap", map[string]uint32{"view": 0})
	_, err = view.SessionVersion(t.Context(), s, "view")
	require.ErrorIs(t, err, view.ErrNoCommonVersion, "the initiator found no common version")

	// An oversized list is refused before it is decoded, and creates no session.
	msg := &view.Message{SessionID: "big", Versions: make([]byte, preamble.MaxVersionsSize+1), Ctx: t.Context()}
	_, err = p.NewResponderSession("big", "ctx", "addr", []byte("peer"), view.Identity("caller"), msg)
	require.ErrorContains(t, err, "exceed")
}
