/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
)

func TestReadPreamble(t *testing.T) {
	t.Parallel()
	testSetup(t)
	serverTLS, clientTLS, _ := testMutualTLSConfigs(t, false)
	newServer := func(header http.Header) string {
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if conn, err := (&gwebsocket.Upgrader{}).Upgrade(w, r, header); err == nil {
				_ = conn.Close()
			}
		}))
		srv.TLS = serverTLS
		srv.StartTLS()
		t.Cleanup(srv.Close)
		return strings.TrimPrefix(srv.URL, "https://")
	}
	peerID := serverPeerID(t, serverTLS)

	raw, err := ReadPreamble(t.Context(), peerID, newServer(localPreambleHeader()), clientTLS)
	require.NoError(t, err)
	require.Equal(t, preamble.LocalBytes(), raw)

	// A peer of another epoch is reached; its preamble is returned for the caller to refuse.
	other := encodedPreamble(t, preamble.FramingEpoch+1)
	raw, err = ReadPreamble(t.Context(), peerID, newServer(http.Header{preambleHeader: []string{base64.StdEncoding.EncodeToString(other)}}), clientTLS)
	require.NoError(t, err)
	require.Equal(t, other, raw)

	raw, err = ReadPreamble(t.Context(), peerID, newServer(nil), clientTLS)
	require.NoError(t, err)
	require.Nil(t, raw, "a peer of a release without preambles")

	_, err = ReadPreamble(t.Context(), peerID, newServer(http.Header{preambleHeader: []string{"not base64!"}}), clientTLS)
	require.ErrorIs(t, err, ErrBadPreamble)
	require.NotContains(t, err.Error(), "\n", "the error is printed on one line")

	_, err = ReadPreamble(t.Context(), "someone else", newServer(localPreambleHeader()), clientTLS)
	require.ErrorContains(t, err, "peer identity binding failed")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	_, err = ReadPreamble(t.Context(), peerID, addr, clientTLS)
	require.ErrorContains(t, err, "failed to open websocket")

	_, err = ReadPreamble(t.Context(), peerID, addr, nil)
	require.ErrorContains(t, err, "requires TLS")
}
