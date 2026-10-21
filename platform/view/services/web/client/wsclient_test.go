/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wsUpgrader is a shared test upgrader with permissive origin checking.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// toWsURL converts an httptest.Server URL from "http://..." to "ws://...".
func toWsURL(serverURL, path string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + path
}

// wsEchoHandler upgrades to WebSocket and reads messages until the client disconnects.
// Used to keep a connection alive for tests that only need a connected stream.
func wsEchoHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// --- OpenWSClientConn ---

// Verifies that OpenWSClientConn establishes a connection to a valid
// WebSocket server and returns a usable *websocket.Conn.
func TestOpenWSClientConn_Connects_To_Valid_Server(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	conn, err := OpenWSClientConn(toWsURL(server.URL, ""), nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer func() { _ = conn.Close() }()
}

// Verifies that OpenWSClientConn returns an error when the server is unreachable.
func TestOpenWSClientConn_Fails_When_Server_Unreachable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	server.Close()

	conn, err := OpenWSClientConn(toWsURL(server.URL, ""), nil)
	require.Error(t, err)
	require.Nil(t, conn)
}

// Verifies that OpenWSClientConnContext times out when the context deadline expires while connecting
// to a server that accepts TCP connections but never completes the WebSocket handshake.
func TestOpenWSClientConnContext_Deadline(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer func() { _ = conn.Close() }()
		}
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	conn, err := OpenWSClientConnContext(ctx, "ws://"+listener.Addr().String(), nil)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Nil(t, conn)
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Less(t, elapsed, 2*time.Second)
}

// Verifies that OpenWSClientConnContext returns an error immediately when called with a cancelled context.
func TestOpenWSClientConnContext_Cancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	conn, err := OpenWSClientConnContext(ctx, "ws://127.0.0.1:0", nil)
	require.Error(t, err)
	require.Nil(t, conn)
}
