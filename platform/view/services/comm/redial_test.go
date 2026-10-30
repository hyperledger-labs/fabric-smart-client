/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// retiringHost records the peers it was asked to retire.
type retiringHost struct {
	mockHost
	mu      sync.Mutex
	retired []host.PeerID
}

func (h *retiringHost) Retire(peerID host.PeerID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.retired = append(h.retired, peerID)
}

func (h *retiringHost) get() []host.PeerID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]host.PeerID(nil), h.retired...)
}

// A draining peer's refusal of a session this node opened retires the host's connections to
// that peer, so the next session dials again, and still reaches the session; another error,
// or a refusal on a session this node does not know, retires nothing.
func TestRedialAfterDraining(t *testing.T) {
	t.Parallel()
	h := &retiringHost{}
	p, err := NewNode(t.Context(), h, &disabled.Provider{})
	require.NoError(t, err)
	stopNode(t, p)
	p.Start(t.Context())

	s, err := p.NewSessionWithID("s", "ctx", "ep", []byte("bob"))
	require.NoError(t, err)
	deliver := func(sessionID string, status int32, payload string) {
		p.incomingMessages <- &messageWithStream{
			message: &view.Message{SessionID: sessionID, FromPKID: []byte("bob"), Status: status, Payload: []byte(payload)},
			stream:  &streamHandler{stream: &mockStream{ctx: t.Context()}, node: p},
		}
	}

	deliver("s", view.ERROR, "busy: too many concurrent responders for this peer")
	<-s.Receive()
	deliver("unknown", view.ERROR, DrainingPrefix+"node is draining")
	deliver("s", view.OK, DrainingPrefix+"a payload, not a refusal")
	<-s.Receive()
	assert.Empty(t, h.get())

	deliver("s", view.ERROR, DrainingPrefix+"node is draining")
	select {
	case msg := <-s.Receive():
		assert.Equal(t, DrainingPrefix+"node is draining", string(msg.Payload))
	case <-time.After(3 * time.Second):
		t.Fatal("the refusal did not reach the session")
	}
	assert.Equal(t, []host.PeerID{"bob"}, h.get())
}
