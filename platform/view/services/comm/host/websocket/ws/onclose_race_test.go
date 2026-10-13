/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gwebsocket "github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// TestOnCloseRace dials a server that drops the websocket immediately. onClose must not
// read the client connection before getClientConn has assigned and cached it.
func TestOnCloseRace(t *testing.T) {
	up := gwebsocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err == nil {
			_ = c.Close()
		}
	}))
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")
	for range 20 {
		p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
		_, _ = p.NewClientStream(host.StreamInfo{RemotePeerAddress: addr}, t.Context(), "client", nil)
		_ = p.Close()
	}
}
