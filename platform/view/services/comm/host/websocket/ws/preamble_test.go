/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

func encodedPreamble(t *testing.T, epoch uint32) []byte {
	t.Helper()
	p := preamble.Local()
	p.FramingEpoch = epoch
	raw, err := preamble.Encode(p)
	require.NoError(t, err)
	return raw
}

// Both ends of a stream see the other's preamble: the server from StreamMeta, the client from
// the upgrade response.
func TestPreambleExchange(t *testing.T) {
	t.Parallel()
	testSetup(t)
	serverTLS, clientTLS, clientID := testMutualTLSConfigs(t, false)
	newProvider := func() *MultiplexedProvider {
		p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
		t.Cleanup(func() { _ = p.Close() })
		return p
	}
	streams := make(chan host.P2PStream, 1)
	srv := startTestServer(t, newProvider(), serverTLS, func(s host.P2PStream) { streams <- s })
	t.Cleanup(srv.Close)

	s, err := newProvider().NewClientStream(host.StreamInfo{
		RemotePeerID:      serverPeerID(t, serverTLS),
		RemotePeerAddress: strings.TrimPrefix(srv.URL, "https://"),
		SessionID:         "s",
	}, t.Context(), clientID, clientTLS)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	want := preamble.Local()
	requirePreamble(t, want, s)
	requirePreamble(t, want, receiveStream(t, streams))
}

func requirePreamble(t *testing.T, want *preamble.Preamble, s host.P2PStream) {
	t.Helper()
	ps, ok := s.(host.PreambleStream)
	require.True(t, ok)
	require.NotNil(t, ps.Preamble())
	require.Equal(t, want.GetFramingEpoch(), ps.Preamble().GetFramingEpoch())
	require.Len(t, ps.Preamble().GetRanges(), len(want.GetRanges()))
}

// A sub-connection whose preamble names another framing epoch is refused with a readable
// version error; one without a preamble, from an older release, is accepted.
func TestServerChecksClientPreamble(t *testing.T) {
	t.Parallel()
	testSetup(t)
	url, streams, clientTLS, clientID := startMultiplexedServer(t, 0)
	r := dialRaw(t, url, clientTLS)

	r.send(MultiplexedMessage{ID: "1", Msg: mustMarshal(StreamMeta{SessionID: "s1", PeerID: clientID, Preamble: encodedPreamble(t, preamble.FramingEpoch+1)})})
	mm := r.next()
	require.Equal(t, "1", mm.ID)
	require.Contains(t, mm.Err, "version: ")
	require.Contains(t, mm.Err, "peer framing epoch 2, local 1")

	// The connection stays usable for sub-connections that pass.
	r.sendMeta("2", clientID)
	s := receiveStream(t, streams)
	require.Nil(t, s.(host.PreambleStream).Preamble(), "a peer without a preamble is recorded as such")

	r.send(MultiplexedMessage{ID: "3", Msg: mustMarshal(StreamMeta{SessionID: "s3", PeerID: clientID, Preamble: encodedPreamble(t, preamble.FramingEpoch)})})
	requirePreamble(t, preamble.Local(), receiveStream(t, streams))
}

// The client refuses a server whose preamble names another framing epoch, and accepts one that
// sends no preamble.
func TestClientChecksServerPreamble(t *testing.T) {
	t.Parallel()
	testSetup(t)
	serverTLS, clientTLS, clientID := testMutualTLSConfigs(t, false)
	newServer := func(header http.Header) *httptest.Server {
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := (&gwebsocket.Upgrader{}).Upgrade(w, r, header)
			if err == nil {
				<-r.Context().Done()
				_ = conn.Close()
			}
		}))
		srv.TLS = serverTLS
		srv.StartTLS()
		t.Cleanup(srv.Close)
		return srv
	}
	dial := func(srv *httptest.Server) (host.P2PStream, error) {
		p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
		t.Cleanup(func() { _ = p.Close() })
		return p.NewClientStream(host.StreamInfo{
			RemotePeerID:      serverPeerID(t, serverTLS),
			RemotePeerAddress: strings.TrimPrefix(srv.URL, "https://"),
		}, t.Context(), clientID, clientTLS)
	}

	other := newServer(http.Header{preambleHeader: []string{base64.StdEncoding.EncodeToString(encodedPreamble(t, preamble.FramingEpoch+1))}})
	_, err := dial(other)
	require.ErrorIs(t, err, preamble.ErrEpoch)
	require.ErrorContains(t, err, "version: ")
	require.ErrorContains(t, err, "peer framing epoch 2, local 1")

	for name, value := range map[string]string{
		"oversized": strings.Repeat("A", base64.StdEncoding.EncodedLen(preamble.MaxSize)+4),
		"malformed": "not base64!",
	} {
		_, err := dial(newServer(http.Header{preambleHeader: []string{value}}))
		require.ErrorContains(t, err, "version: ", name)
	}

	old := newServer(nil)
	s, err := dial(old)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.Nil(t, s.(host.PreambleStream).Preamble())
}
