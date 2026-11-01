/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"encoding/json"
	"testing"
	"time"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

// FuzzMultiplexedMessage fuzzes the server side of a multiplexed websocket connection with
// one message from a peer: readIncoming decodes it with readMultiplexedMessage and opens,
// feeds or closes a sub-connection for it. The connection either serves the message, so a
// valid frame sent after it still opens a sub-connection, or closes.
func FuzzMultiplexedMessage(f *testing.F) {
	const peerID host.PeerID = "peer"
	meta := mustMarshal(StreamMeta{PeerID: peerID, SessionID: "s1", ContextID: "c1"})
	probe := mustMarshal(StreamMeta{PeerID: peerID, SessionID: "probe", ContextID: "probe"})

	f.Add(mustMarshal(MultiplexedMessage{ID: "1", Msg: meta}))
	f.Add(mustMarshal(MultiplexedMessage{ID: "1", Msg: mustMarshal(StreamMeta{PeerID: "spoofed"})}))
	f.Add(mustMarshal(MultiplexedMessage{ID: "1", Err: "closed"}))
	f.Add(mustMarshal(MultiplexedMessage{ID: "1", Msg: []byte("not json")}))
	f.Add([]byte(`{"id":"1","msg":"bm90IGJhc2U2NA=="}`))
	f.Add([]byte("not json"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, raw []byte) {
		// A fuzzed message with ID "probe" turns the probe below into data for its own
		// sub-connection, so no new stream opens. Decode with the server's decoder so the
		// check matches the ID the server sees.
		var mm MultiplexedMessage
		if json.Unmarshal(raw, &mm) == nil && mm.ID == "probe" {
			return
		}

		streams := make(chan host.P2PStream, 2)
		sc, peer := newPipedServerConn(t, peerID, 2, func(s host.P2PStream) { streams <- s })
		t.Cleanup(func() {
			_ = peer.Close()
			_ = sc.Kill()
			for {
				select {
				case s := <-streams:
					_ = s.Close()
				default:
					return
				}
			}
		})

		// The peer reads every frame the server writes back, so a rejection never blocks the
		// read loop on the pipe.
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			for {
				if _, _, err := peer.ReadMessage(); err != nil {
					return
				}
			}
		}()

		require.NoError(t, peer.WriteMessage(gwebsocket.TextMessage, raw))
		// ID "probe" is unused by the fuzzed message, so the server opens a sub-connection for
		// it unless the fuzzed message made it close the connection. The fuzzed message may
		// open a sub-connection of its own first, and the cap of 2 leaves room for both.
		if err := peer.WriteMessage(gwebsocket.TextMessage, mustMarshal(MultiplexedMessage{ID: "probe", Msg: probe})); err != nil {
			<-closed
			return
		}
		deadline := time.After(5 * time.Second)
		for {
			select {
			case s := <-streams:
				if s.(*stream).ContextID() == "probe" {
					return
				}
			case <-closed:
				return
			case <-deadline:
				t.Fatal("server neither served the probe nor closed the connection")
			}
		}
	})
}
