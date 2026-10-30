/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// drainState drains always; its responder context "known" has the party "alice".
type drainState struct{}

func (drainState) Draining() bool { return true }

func (drainState) IsParty(contextID string, pkid []byte) bool {
	return contextID == "known" && string(pkid) == "alice"
}

func (drainState) Track() func() { return func() {} }

func TestService_DrainingAdmission(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, contextID, peer, caller string
		admitted                      bool
		reply                         string
	}{
		{name: "new context", contextID: "new", peer: "alice", caller: "hosted", reply: p2p.DrainingPrefix + "node is draining"},
		{name: "known context, same peer", contextID: "known", peer: "alice", caller: "hosted", admitted: true},
		{name: "known context, other peer", contextID: "known", peer: "mallory", caller: "hosted", reply: p2p.DrainingPrefix + "node is draining"},
		{name: "unknown view", contextID: "new", peer: "alice", caller: "unhosted", reply: "unknown_view: unhosted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := newRejectTest(t, 10, p2p.WithDrainState(drainState{}))
			rt.sendMsg(&view.Message{ContextID: tc.contextID, SessionID: "s", Caller: tc.caller, FromPKID: []byte(tc.peer)})
			if tc.admitted {
				rt.wait(rt.runner.entered, "the responder to run")
				close(rt.runner.release(tc.contextID))
				require.Zero(t, rt.cl.ReplyErrorCallCount())
				return
			}
			require.Eventually(t, func() bool { return rt.cl.ReplyErrorCallCount() == 1 }, 5*time.Second, 5*time.Millisecond)
			_, _, payload := rt.cl.ReplyErrorArgsForCall(0)
			require.Equal(t, tc.reply, string(payload))
			require.Zero(t, rt.cl.NewResponderSessionCallCount())
		})
	}
}

// countingDrain tracks responders and never drains.
type countingDrain struct {
	drainState
	running atomic.Int32
}

func (*countingDrain) Draining() bool { return false }

func (d *countingDrain) Track() func() {
	d.running.Add(1)
	return func() { d.running.Add(-1) }
}

// A responder counts as running from the moment dispatch admits its first message, before it has
// a view context, so a drain that starts in between waits for it.
func TestService_TracksRespondersFromAdmission(t *testing.T) {
	t.Parallel()
	d := &countingDrain{}
	rt := newRejectTest(t, 10, p2p.WithDrainState(d))
	rt.sendMsg(&view.Message{ContextID: "c", SessionID: "s", Caller: "hosted"})
	rt.wait(rt.runner.entered, "the responder to run")
	require.Equal(t, int32(1), d.running.Load())
	close(rt.runner.release("c"))
	require.Eventually(t, func() bool { return d.running.Load() == 0 }, 5*time.Second, 5*time.Millisecond)
}
