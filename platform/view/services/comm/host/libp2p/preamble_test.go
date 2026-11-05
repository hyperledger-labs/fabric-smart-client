/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	commhost "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// The view packet written after the preamble frame is still on the stream once the frame is read.
func TestPreambleFrame_LeavesFollowingBytes(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	require.NoError(t, writePreambleFrame(&buf, preamble.LocalBytes()))
	buf.WriteString("first view packet")

	raw, err := readPreambleFrame(&buf)
	require.NoError(t, err)
	require.Equal(t, preamble.LocalBytes(), raw)
	require.Equal(t, "first view packet", buf.String())
}

func TestPreambleFrame_RefusesOversizedBeforeAllocating(t *testing.T) {
	t.Parallel()
	_, err := readPreambleFrame(bytes.NewReader(binary.AppendUvarint(nil, 1<<40)))
	require.ErrorIs(t, err, errFrameTooLarge)
}

// A frame over preamble.MaxSize is a refusal: reported as version and counted.
func TestPreambleRefusesOversizedFrame(t *testing.T) {
	t.Parallel()
	dialer, listener, _ := twoHosts(t)
	listener.SetStreamHandler(viewProtocolV2, func(s network.Stream) {
		_, _ = s.Write(binary.AppendUvarint(nil, preamble.MaxSize+1))
		<-t.Context().Done()
	})

	_, err := dialer.NewStream(t.Context(), commhost.StreamInfo{RemotePeerID: listener.PeerID()})
	require.ErrorIs(t, err, errFrameTooLarge)
	require.ErrorContains(t, err, "version: ")
}

// twoHosts starts two libp2p hosts; streams accepted by the second are sent on the returned
// channel. The first knows the second's address.
func twoHosts(t *testing.T) (dialer, listener *host, accepted chan commhost.P2PStream) {
	t.Helper()
	addrs := freeLibP2PAddresses(t, 2)
	newHost := func(addr string) (*host, string) {
		sk, id := generateKey(t)
		c := &mock.LibP2PConfig{}
		c.ListenAddressReturns(addr)
		h, err := newLibP2PHost(c, sk, newMetrics(&disabled.Provider{}))
		require.NoError(t, err)
		t.Cleanup(func() { _ = h.Close() })
		return h, id
	}
	dialer, _ = newHost(addrs[0])
	listener, listenerID := newHost(addrs[1])
	accepted = make(chan commhost.P2PStream, 1)
	require.NoError(t, listener.Start(func(s commhost.P2PStream) { accepted <- s }))

	pid, err := peer.Decode(listenerID)
	require.NoError(t, err)
	ma, err := multiaddr.NewMultiaddr(addrs[1])
	require.NoError(t, err)
	dialer.Peerstore().AddAddr(pid, ma, time.Hour)
	return dialer, listener, accepted
}

func TestPreambleExchange(t *testing.T) {
	t.Parallel()
	dialer, listener, accepted := twoHosts(t)

	s, err := dialer.NewStream(t.Context(), commhost.StreamInfo{RemotePeerID: listener.PeerID()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.Equal(t, preamble.FramingEpoch, s.(commhost.PreambleStream).Preamble().GetFramingEpoch())

	// The listener sees the dialer's preamble too, and the first message arrives intact.
	_, err = s.Write([]byte("hello"))
	require.NoError(t, err)
	in := <-accepted
	require.Equal(t, preamble.FramingEpoch, in.(commhost.PreambleStream).Preamble().GetFramingEpoch())
	got := make([]byte, 5)
	_, err = in.Read(got)
	require.NoError(t, err)
	require.Equal(t, "hello", string(got))
}

// A peer of a release that registers only viewProtocol still connects, without a preamble.
func TestPreambleFallbackToV1(t *testing.T) {
	t.Parallel()
	dialer, listener, accepted := twoHosts(t)
	listener.RemoveStreamHandler(viewProtocolV2)

	s, err := dialer.NewStream(t.Context(), commhost.StreamInfo{RemotePeerID: listener.PeerID()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.Nil(t, s.(commhost.PreambleStream).Preamble())
	_, err = s.Write([]byte("x"))
	require.NoError(t, err)
	require.Nil(t, (<-accepted).(commhost.PreambleStream).Preamble())
}

// A peer that negotiates viewProtocolV2 but never sends its frame does not hold the dialer
// past the caller's context.
func TestPreambleHonoursContext(t *testing.T) {
	t.Parallel()
	dialer, listener, _ := twoHosts(t)
	listener.SetStreamHandler(viewProtocolV2, func(network.Stream) { <-t.Context().Done() })

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := dialer.NewStream(ctx, commhost.StreamInfo{RemotePeerID: listener.PeerID()})
	require.Error(t, err)
	require.Less(t, time.Since(start), 5*time.Second)
}

// A peer on another framing epoch is refused; the dialer gets an error naming both epochs.
func TestPreambleRefusesOtherEpoch(t *testing.T) {
	t.Parallel()
	dialer, listener, accepted := twoHosts(t)
	other := preamble.Local()
	other.FramingEpoch = preamble.FramingEpoch + 1
	raw, err := preamble.Encode(other)
	require.NoError(t, err)
	listener.SetStreamHandler(viewProtocolV2, func(s network.Stream) {
		_ = writePreambleFrame(s, raw)
		_, _ = readPreambleFrame(s)
	})

	_, err = dialer.NewStream(t.Context(), commhost.StreamInfo{RemotePeerID: listener.PeerID()})
	require.ErrorIs(t, err, preamble.ErrEpoch)
	require.ErrorContains(t, err, "version: ")
	require.ErrorContains(t, err, "peer framing epoch 2, local 1")
	require.Empty(t, accepted)
}
