/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type recipientsEnv struct {
	ctx     *mockViewContext
	session *mockSession
	store   *testBindingStore
}

// newRecipientsEnv wires a view context whose session is session and whose
// services are nsp (absent when nil) and an endpoint service backed by an
// in-memory binding store.
func newRecipientsEnv(t *testing.T, session *mockSession, nsp *fabric.NetworkServiceProvider) *recipientsEnv {
	t.Helper()
	store := newTestBindingStore()
	es, err := endpoint.NewService(store)
	require.NoError(t, err)
	ctx := &mockViewContext{
		session: session,
		getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
			return session, nil
		},
		getServiceFn: func(v any) (any, error) {
			switch v {
			case reflect.TypeFor[*fabric.NetworkServiceProvider]():
				if nsp != nil {
					return nsp, nil
				}
			case reflect.TypeFor[*endpoint.Service]():
				return es, nil
			}
			return nil, errors.New("service missing")
		},
	}
	return &recipientsEnv{ctx: ctx, session: session, store: store}
}

func recipientsNSP() *fabric.NetworkServiceProvider {
	return newTestFabricNetworkServiceProvider(view.Identity("me-id"), &mockDriverChannel{name: "ch"})
}

func failingNSP() *fabric.NetworkServiceProvider {
	return fabric.NewNetworkServiceProvider(&testFabricDriverFNSProvider{err: errors.New("fns failed")}, nil)
}

func noDefaultNSP() *fabric.NetworkServiceProvider {
	return newTestFabricNetworkServiceProvider(nil, &mockDriverChannel{name: "ch"})
}

// newRecv returns a closed channel that delivers msgs.
func newRecv(msgs ...*view.Message) <-chan *view.Message {
	ch := make(chan *view.Message, len(msgs))
	for _, m := range msgs {
		ch <- m
	}
	close(ch)
	return ch
}

func okMsg(t *testing.T, payload interface{ Bytes() ([]byte, error) }) *view.Message {
	t.Helper()
	raw, err := payload.Bytes()
	require.NoError(t, err)
	return &view.Message{Status: view.OK, Payload: raw}
}

func TestExchangeRecipientIdentitiesViewCall(t *testing.T) {
	t.Parallel()

	reply := func(t *testing.T) *view.Message {
		t.Helper()
		return okMsg(t, &RecipientData{Identity: view.Identity("other-recipient")})
	}

	for _, tc := range []struct {
		name  string
		label string
		me    view.Identity
	}{
		{name: "default identity", me: view.Identity("me-id")},
		{name: "identity by label", label: "bob", me: view.Identity("bob-id")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newRecipientsEnv(t, &mockSession{recv: newRecv(reply(t))}, recipientsNSP())

			out, err := (&ExchangeRecipientIdentitiesView{Other: view.Identity("other"), IdentityLabel: tc.label}).Call(env.ctx)
			require.NoError(t, err)
			require.Equal(t, []view.Identity{tc.me, view.Identity("other-recipient")}, out)

			require.Len(t, env.session.sent, 1)
			req := &ExchangeRecipientRequest{}
			require.NoError(t, req.FromBytes(env.session.sent[0]))
			require.Equal(t, []byte("other"), req.WalletID)
			require.Equal(t, tc.me, req.RecipientData.Identity)

			require.Equal(t, map[string]view.Identity{
				"other-recipient": view.Identity("other"),
				string(tc.me):     view.Identity("me"),
			}, env.store.bindings)
		})
	}

	for _, tc := range []struct {
		name     string
		label    string
		nsp      func() *fabric.NetworkServiceProvider
		session  func(t *testing.T) *mockSession
		failBind view.Identity
		sent     int
		wantErr  string
	}{
		{name: "fns error", nsp: failingNSP, wantErr: "fns failed"},
		{name: "unknown label", label: "carol", wantErr: "failed to get identity with label carol"},
		{name: "no default identity", nsp: noDefaultNSP, wantErr: "no identity found with label"},
		{
			name:    "send error",
			session: func(*testing.T) *mockSession { return &mockSession{sendErr: errors.New("send failed")} },
			wantErr: "send failed",
		},
		{
			name:    "closed reply channel",
			session: func(*testing.T) *mockSession { return &mockSession{recv: newRecv()} },
			sent:    1,
			wantErr: "session receive channel is closed",
		},
		{
			name: "error reply",
			session: func(*testing.T) *mockSession {
				return &mockSession{recv: newRecv(&view.Message{Status: view.ERROR, Payload: []byte("boom")})}
			},
			sent:    1,
			wantErr: "received error from remote [boom]",
		},
		{
			name: "malformed reply",
			session: func(*testing.T) *mockSession {
				return &mockSession{recv: newRecv(&view.Message{Status: view.OK, Payload: []byte("{bad")})}
			},
			sent:    1,
			wantErr: "invalid character",
		},
		{name: "bind other error", failBind: view.Identity("other"), sent: 1, wantErr: "failed storing bindings"},
		{name: "bind me error", failBind: view.Identity("me"), sent: 1, wantErr: "failed storing bindings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nsp := recipientsNSP()
			if tc.nsp != nil {
				nsp = tc.nsp()
			}
			session := &mockSession{recv: newRecv(reply(t))}
			if tc.session != nil {
				session = tc.session(t)
			}
			env := newRecipientsEnv(t, session, nsp)
			env.store.failLongTerm = tc.failBind

			_, err := (&ExchangeRecipientIdentitiesView{Other: view.Identity("other"), IdentityLabel: tc.label}).Call(env.ctx)
			require.ErrorContains(t, err, tc.wantErr)
			require.Len(t, env.session.sent, tc.sent)
		})
	}
}

func TestRespondExchangeRecipientIdentitiesViewCall(t *testing.T) {
	t.Parallel()

	request := func(t *testing.T) *view.Message {
		t.Helper()
		return okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: view.Identity("other-recipient")}})
	}
	caller := view.SessionInfo{Caller: view.Identity("caller")}

	for _, tc := range []struct {
		name  string
		label string
		me    view.Identity
	}{
		{name: "default identity", me: view.Identity("me-id")},
		{name: "identity by label", label: "bob", me: view.Identity("bob-id")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newRecipientsEnv(t, &mockSession{info: caller, recv: newRecv(request(t))}, recipientsNSP())

			out, err := (&RespondExchangeRecipientIdentitiesView{IdentityLabel: tc.label}).Call(env.ctx)
			require.NoError(t, err)
			require.Equal(t, []view.Identity{tc.me, view.Identity("other-recipient")}, out)

			require.Len(t, env.session.sent, 1)
			data := &RecipientData{}
			require.NoError(t, data.FromBytes(env.session.sent[0]))
			require.Equal(t, tc.me, data.Identity)

			require.Equal(t, map[string]view.Identity{
				string(tc.me):     view.Identity("me"),
				"other-recipient": view.Identity("caller"),
			}, env.store.bindings)
		})
	}

	for _, tc := range []struct {
		name     string
		label    string
		nsp      func() *fabric.NetworkServiceProvider
		sendErr  error
		failBind view.Identity
		sent     int
		wantErr  string
	}{
		{name: "fns error", nsp: failingNSP, wantErr: "fns failed"},
		{name: "unknown label", label: "carol", wantErr: "failed to get identity with label carol"},
		{name: "no default identity", nsp: noDefaultNSP, wantErr: "no identity found with label"},
		{name: "send error", sendErr: errors.New("send failed"), wantErr: "send failed"},
		{name: "bind me error", failBind: view.Identity("me"), sent: 1, wantErr: "failed storing bindings"},
		{name: "bind caller error", failBind: view.Identity("caller"), sent: 1, wantErr: "failed storing bindings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nsp := recipientsNSP()
			if tc.nsp != nil {
				nsp = tc.nsp()
			}
			env := newRecipientsEnv(t, &mockSession{info: caller, recv: newRecv(request(t)), sendErr: tc.sendErr}, nsp)
			env.store.failLongTerm = tc.failBind

			_, err := (&RespondExchangeRecipientIdentitiesView{IdentityLabel: tc.label}).Call(env.ctx)
			require.ErrorContains(t, err, tc.wantErr)
			require.Len(t, env.session.sent, tc.sent)
		})
	}
}

func TestRequestRecipientIdentityViewBindings(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		failBind view.Identity
		wantErr  string
	}{
		{name: "binds recipient to other"},
		{name: "bind error", failBind: view.Identity("other"), wantErr: "failed storing bindings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reply := okMsg(t, &RecipientData{Identity: view.Identity("other-recipient")})
			env := newRecipientsEnv(t, &mockSession{recv: newRecv(reply)}, nil)
			env.store.failLongTerm = tc.failBind

			out, err := (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, view.Identity("other-recipient"), out)
			require.Equal(t, map[string]view.Identity{"other-recipient": view.Identity("other")}, env.store.bindings)
		})
	}
}

func TestRequestRecipientIdentityViewBadReply(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		recv    <-chan *view.Message
		wantErr string
	}{
		{name: "closed reply channel", recv: newRecv(), wantErr: "session receive channel is closed"},
		{
			name:    "error reply",
			recv:    newRecv(&view.Message{Status: view.ERROR, Payload: []byte(`{"Identity":"b3RoZXI="}`)}),
			wantErr: "received error from remote",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newRecipientsEnv(t, &mockSession{recv: tc.recv}, nil)

			var err error
			require.NotPanics(t, func() {
				_, err = (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
			})
			require.ErrorContains(t, err, tc.wantErr)
			require.Empty(t, env.store.bindings, "nothing may be bound on a bad reply")
		})
	}
}

func TestRespondRequestRecipientIdentityViewCall(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		identity view.Identity
		nsp      func() *fabric.NetworkServiceProvider
		sendErr  error
		failBind view.Identity
		want     view.Identity
		wantErr  string
	}{
		// A preset identity needs no network service: none is registered.
		{name: "preset identity", identity: view.Identity("preset"), want: view.Identity("preset")},
		{name: "default identity", nsp: recipientsNSP, want: view.Identity("me-id")},
		{name: "fns error", nsp: failingNSP, wantErr: "fns failed"},
		{name: "send error", identity: view.Identity("preset"), sendErr: errors.New("send failed"), wantErr: "send failed"},
		{name: "bind error", identity: view.Identity("preset"), failBind: view.Identity("me"), wantErr: "failed storing bindings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var nsp *fabric.NetworkServiceProvider
			if tc.nsp != nil {
				nsp = tc.nsp()
			}
			env := newRecipientsEnv(t, &mockSession{recv: newRecv(okMsg(t, &RecipientRequest{})), sendErr: tc.sendErr}, nsp)
			env.store.failLongTerm = tc.failBind

			out, err := (&RespondRequestRecipientIdentityView{Identity: tc.identity}).Call(env.ctx)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, out)

			require.Len(t, env.session.sent, 1)
			data := &RecipientData{}
			require.NoError(t, data.FromBytes(env.session.sent[0]))
			require.Equal(t, tc.want, data.Identity)
			require.Equal(t, map[string]view.Identity{string(tc.want): view.Identity("me")}, env.store.bindings)
		})
	}
}

func TestRecipientWrappersErrors(t *testing.T) {
	t.Parallel()

	failingOpt := func(*ServiceOptions) error { return errors.New("bad option") }
	wrappers := []struct {
		name string
		call func(ctx view.Context, opts ...ServiceOption) error
	}{
		{name: "RequestRecipientIdentity", call: func(ctx view.Context, _ ...ServiceOption) error {
			_, err := RequestRecipientIdentity(ctx, view.Identity("other"))
			return err
		}},
		{name: "RespondRequestRecipientIdentity", call: func(ctx view.Context, _ ...ServiceOption) error {
			_, err := RespondRequestRecipientIdentity(ctx)
			return err
		}},
		{name: "ExchangeRecipientIdentities", call: func(ctx view.Context, opts ...ServiceOption) error {
			_, _, err := ExchangeRecipientIdentities(ctx, view.Identity("other"), opts...)
			return err
		}},
		{name: "RespondExchangeRecipientIdentities", call: func(ctx view.Context, opts ...ServiceOption) error {
			_, _, err := RespondExchangeRecipientIdentities(ctx, opts...)
			return err
		}},
	}

	for _, w := range wrappers {
		t.Run(w.name+" run view error", func(t *testing.T) {
			t.Parallel()
			expected := errors.New("run failed")
			ctx := &mockViewContext{runViewFn: func(view.View, ...view.RunViewOption) (any, error) { return nil, expected }}
			require.ErrorIs(t, w.call(ctx), expected)
		})

		t.Run(w.name+" unexpected result type", func(t *testing.T) {
			t.Parallel()
			ctx := &mockViewContext{runViewFn: func(view.View, ...view.RunViewOption) (any, error) { return 42, nil }}
			require.EqualError(t, w.call(ctx), "unexpected view result type [int]")
		})
	}

	for _, w := range wrappers[2:] {
		t.Run(w.name+" option error", func(t *testing.T) {
			t.Parallel()
			ran := false
			ctx := &mockViewContext{runViewFn: func(view.View, ...view.RunViewOption) (any, error) {
				ran = true
				return nil, nil
			}}
			err := w.call(ctx, failingOpt)
			require.ErrorContains(t, err, "failed to compile service options")
			require.ErrorContains(t, err, "bad option")
			require.False(t, ran)
		})

		t.Run(w.name+" short result", func(t *testing.T) {
			t.Parallel()
			ctx := &mockViewContext{runViewFn: func(view.View, ...view.RunViewOption) (any, error) {
				return []view.Identity{view.Identity("only-one")}, nil
			}}
			var err error
			require.NotPanics(t, func() { err = w.call(ctx) })
			require.EqualError(t, err, "expected 2 identities, got [1]")
		})
	}
}
