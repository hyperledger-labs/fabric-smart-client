/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestService_UnsupportedVersionIsRejected checks that a first message stating a component
// version outside this node's range (view at {1, 1}) gets no_common_version and no responder,
// and that a component this node does not have is not checked.
func TestService_UnsupportedVersionIsRejected(t *testing.T) {
	t.Parallel()
	rt := newRejectTest(t, 1)

	rt.sendMsg(&view.Message{ContextID: "ctx", SessionID: "sess", Caller: "hosted", Versions: versions(t, map[string]uint32{"view": 2})})

	require.Eventually(t, func() bool { return rt.cl.ReplyErrorCallCount() == 1 }, 5*time.Second, 5*time.Millisecond)
	_, _, payload := rt.cl.ReplyErrorArgsForCall(0)
	require.Equal(t, "no_common_version: view 2 not in [1, 1]", string(payload))
	require.Zero(t, rt.cl.NewResponderSessionCallCount())
	require.Eventually(t, func() bool { return rt.counter.count("reason=no_common_version") == 1 }, 5*time.Second, 5*time.Millisecond)

	rt.sendMsg(&view.Message{ContextID: "ctx2", SessionID: "sess2", Caller: "hosted", Versions: versions(t, map[string]uint32{"view": 1, "unknown": 9})})
	rt.wait(rt.runner.entered, "the responder to run")
	require.Equal(t, 1, rt.cl.ReplyErrorCallCount())
}

func versions(t *testing.T, m map[string]uint32) []byte {
	t.Helper()
	raw, err := preamble.EncodeVersions(m)
	require.NoError(t, err)
	return raw
}

// TestService_NoCommonVersionIsRejected checks that a first message stating 0, no common
// version, for a component this node has, or a list that does not decode, is rejected.
func TestService_NoCommonVersionIsRejected(t *testing.T) {
	t.Parallel()
	rt := newRejectTest(t, 2)

	rt.sendMsg(&view.Message{ContextID: "ctx", SessionID: "sess", Caller: "hosted", Versions: versions(t, map[string]uint32{"view": 0})})
	rt.sendMsg(&view.Message{ContextID: "ctx2", SessionID: "sess2", Caller: "hosted", Versions: []byte{0xff}})

	require.Eventually(t, func() bool { return rt.cl.ReplyErrorCallCount() == 2 }, 5*time.Second, 5*time.Millisecond)
	var payloads []string
	for i := range 2 {
		_, _, payload := rt.cl.ReplyErrorArgsForCall(i)
		payloads = append(payloads, string(payload))
	}
	require.ElementsMatch(t, []string{"no_common_version: view 0 not in [1, 1]", "no_common_version: invalid versions"}, payloads)
	require.Zero(t, rt.cl.NewResponderSessionCallCount())
}
