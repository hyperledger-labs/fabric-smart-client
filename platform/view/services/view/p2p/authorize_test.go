/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	viewservice "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// securityProvider hands out counter for the security rejections counter.
type securityProvider struct {
	metrics.Provider
	counter *countingCounter
}

func (p *securityProvider) NewCounter(metrics.CounterOpts) metrics.Counter { return p.counter }

// TestService_Authorizer checks what dispatch does with each outcome of a responder's
// authorizer: an accepted caller runs the responder; any other outcome replies before a view
// context or responder session exists, with the given payload, and counts the reason.
func TestService_Authorizer(t *testing.T) {
	t.Parallel()
	denied := viewservice.AuthorizerFunc(func(context.Context, view.Identity) error {
		return errors.Join(errors.New("not on the list"), viewservice.ErrDenied)
	})
	for _, tc := range []struct {
		name       string
		peer       string
		authorizer viewservice.ResponderAuthorizer
		reply      string // empty: the responder runs
		security   float64
	}{
		{name: "no authorizer", authorizer: nil},
		{name: "accepted", authorizer: viewservice.AuthorizerFunc(func(context.Context, view.Identity) error { return nil })},
		{name: "denied", authorizer: denied, reply: "unauthorized: caller may not start this view", security: 1},
		{name: "failed", authorizer: viewservice.AuthorizerFunc(func(context.Context, view.Identity) error {
			return errors.New("db down: secret detail")
		}), reply: "authorizer_error: authorizer failed"},
		{name: "timed out", authorizer: viewservice.AuthorizerFunc(func(ctx context.Context, _ view.Identity) error {
			<-ctx.Done()
			return ctx.Err()
		}), reply: "authorizer_error: authorizer failed"},
		{name: "panicked", authorizer: viewservice.AuthorizerFunc(func(context.Context, view.Identity) error {
			panic("boom")
		}), reply: "authorizer_error: authorizer failed"},
		{name: "unknown caller", peer: "stranger", authorizer: denied, reply: "authorizer_error: authorizer failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sec := newCountingCounter()
			rt := newAuthorizedRejectTest(t, 10, tc.authorizer,
				p2p.WithAuthorizeTimeout(50*time.Millisecond),
				p2p.WithRejections(security.NewRejections(&securityProvider{counter: sec})))
			if tc.peer == "" {
				tc.peer = "peer"
			}
			rt.sendMsg(&view.Message{ContextID: "c", SessionID: "s", Caller: "hosted", FromPKID: []byte(tc.peer)})

			if tc.reply == "" {
				rt.wait(rt.runner.entered, "the responder to run")
				close(rt.runner.release("c"))
				require.Zero(t, rt.cl.ReplyErrorCallCount())
				return
			}
			require.Eventually(t, func() bool { return rt.cl.ReplyErrorCallCount() == 1 }, 5*time.Second, 5*time.Millisecond)
			_, _, payload := rt.cl.ReplyErrorArgsForCall(0)
			require.Equal(t, tc.reply, string(payload))
			require.Zero(t, rt.cl.NewResponderSessionCallCount(), "no session for a refused caller")
			require.Empty(t, rt.runner.entered)
			reason, _, _ := strings.Cut(tc.reply, ":")
			require.Eventually(t, func() bool { return rt.counter.count("reason="+reason) == 1 }, 5*time.Second, 5*time.Millisecond)
			require.InDelta(t, tc.security, sec.count("kind="+string(security.KindUnauthorized)), 0)
		})
	}
}

// TestService_AuthorizerGetsResolvedCaller checks that the authorizer gets the identity the
// endpoint service resolves from the sender's PKID, and that the responder session gets the same
// identity.
func TestService_AuthorizerGetsResolvedCaller(t *testing.T) {
	t.Parallel()
	got := make(chan view.Identity, 1)
	rt := newAuthorizedRejectTest(t, 10, viewservice.AuthorizerFunc(func(_ context.Context, caller view.Identity) error {
		got <- caller
		return nil
	}))
	rt.sendMsg(&view.Message{ContextID: "c", SessionID: "s", Caller: "hosted", FromPKID: []byte("peer")})
	rt.wait(rt.runner.entered, "the responder to run")
	require.Equal(t, view.Identity("caller"), <-got)
	remote, _ := rt.cl.NewResponderSessionArgsForCall(0)
	require.Equal(t, []byte("caller"), remote)
	close(rt.runner.release("c"))
}

// TestService_AuthorizerHoldsResponderSlot checks that a running check takes the peer's
// responder slot, so one peer has at most maxRespondersPerPeer checks running.
func TestService_AuthorizerHoldsResponderSlot(t *testing.T) {
	t.Parallel()
	entered, unblock := make(chan struct{}, 10), make(chan struct{})
	rt := newAuthorizedRejectTest(t, 1, viewservice.AuthorizerFunc(func(context.Context, view.Identity) error {
		entered <- struct{}{}
		<-unblock
		return nil
	}))
	rt.sendMsg(&view.Message{ContextID: "a", SessionID: "a", Caller: "hosted"})
	rt.wait(entered, "the first check")
	rt.sendMsg(&view.Message{ContextID: "b", SessionID: "b", Caller: "hosted"})
	require.Eventually(t, func() bool { return rt.cl.ReplyErrorCallCount() == 1 }, 5*time.Second, 5*time.Millisecond)
	_, _, payload := rt.cl.ReplyErrorArgsForCall(0)
	require.Equal(t, "busy: too many concurrent responders for this peer", string(payload))
	require.Empty(t, entered, "the second check did not start")
	close(unblock)
	rt.wait(rt.runner.entered, "the first responder to run")
	close(rt.runner.release("a"))
}
