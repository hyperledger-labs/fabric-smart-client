/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// wsUpgrader is a shared test upgrader with permissive origin checking.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
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

// --- NewWSStream ---

// Verifies that NewWSStream returns a connected WSStream when the server
// accepts the WebSocket upgrade.
func TestNewWSStream_Returns_Connected_Stream(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	stream, err := NewWSStream(toWsURL(server.URL, ""), nil)
	require.NoError(t, err)
	require.NotNil(t, stream)
	defer func() { _ = stream.Close() }()
}

// Verifies that NewWSStream returns an error when the server does not
// perform a WebSocket upgrade (plain HTTP handler).
func TestNewWSStream_Fails_When_Server_Does_Not_Upgrade(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stream, err := NewWSStream(toWsURL(server.URL, ""), nil)
	require.Error(t, err)
	require.Nil(t, stream)
}

// --- Send / Recv ---

// Verifies that Send writes JSON and Recv reads it back correctly
// through an echo server that reflects messages back to the client.
func TestWSStream_Send_And_Recv_JSON_Round_Trip(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		var msg map[string]string
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		_ = conn.WriteJSON(msg)
	}))
	defer server.Close()

	stream, err := NewWSStream(toWsURL(server.URL, ""), nil)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	sent := map[string]string{"hello": "world"}
	require.NoError(t, stream.Send(sent))

	var received map[string]string
	require.NoError(t, stream.Recv(&received))
	require.Equal(t, sent, received)
}

// --- SendInput / Result ---

// Verifies the typed Input/Output protocol used by StreamCallView:
// SendInput writes Input{Raw: ...} and Result reads Output{Raw: ...}.
func TestWSStream_SendInput_And_Result_Protocol(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		var input Input
		if err := conn.ReadJSON(&input); err != nil {
			return
		}
		_ = conn.WriteJSON(&Output{
			Raw: []byte(strings.ToUpper(string(input.Raw))),
		})
	}))
	defer server.Close()

	stream, err := NewWSStream(toWsURL(server.URL, ""), nil)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	require.NoError(t, stream.SendInput([]byte("hello")))

	result, err := stream.Result()
	require.NoError(t, err)
	require.Equal(t, []byte("HELLO"), result)
}

// --- Close ---

// Verifies that Close terminates the connection and subsequent Recv fails.
func TestWSStream_Close_Makes_Subsequent_Recv_Fail(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	defer server.Close()

	stream, err := NewWSStream(toWsURL(server.URL, ""), nil)
	require.NoError(t, err)

	require.NoError(t, stream.Close())

	var msg map[string]string
	require.Error(t, stream.Recv(&msg), "Recv after Close should fail")
}

// --- Result error path ---

// Verifies that Result returns an error when the server closes the
// connection before sending a response.
func TestWSStream_Result_Fails_When_Server_Closes_Connection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	stream, err := NewWSStream(toWsURL(server.URL, ""), nil)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	result, err := stream.Result()
	require.Error(t, err)
	require.Nil(t, result)
}

// --- StreamCallView ---

// Verifies the full StreamCallView flow end-to-end: connects to
// /v1/Views/Stream/{fid}, sends the input payload via WebSocket,
// and returns a stream that can read the server's response.
func TestStreamCallView_Connects_Sends_Input_And_Returns_Stream(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/Views/Stream/myView", r.URL.Path)

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		var input Input
		if err := conn.ReadJSON(&input); err != nil {
			return
		}
		_ = conn.WriteJSON(&Output{Raw: input.Raw})
	}))
	defer server.Close()

	client := newTestClient(t, server)

	stream, err := client.StreamCallView("myView", []byte("test-payload"))
	require.NoError(t, err)
	require.NotNil(t, stream)
	defer func() { _ = stream.Close() }()

	result, err := stream.Result()
	require.NoError(t, err)
	require.Equal(t, []byte("test-payload"), result)
}

// Verifies that StreamCallView wraps connection failures when the server is down.
func TestStreamCallView_Closed_Server_Returns_Init_Error(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
	client := newTestClient(t, server)
	server.Close()

	stream, err := client.StreamCallView("myView", []byte("input"))
	require.Error(t, err)
	require.Nil(t, stream)
	require.Contains(t, err.Error(), "failed to init web socket stream")
}
