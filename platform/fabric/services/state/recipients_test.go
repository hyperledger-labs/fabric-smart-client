/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fabricmsp "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms"
	kmsdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Node identities of the recipient tests: this node is ownNodeID, the
// initiator's counterparty "other" resolves to otherNodeID, and a responder's
// caller is callerID.
var (
	ownNodeID   = view.Identity("own-node")
	otherNodeID = view.Identity("other-node")
	callerID    = view.Identity("caller")
	// noVerifierID signs, but testKeys has no verifier for it, like an idemix
	// identity from an issuer this node holds no credential for.
	noVerifierID = view.Identity("no-verifier")
)

// sessionID is the ID of every recipient test session.
const sessionID = "session"

// testKeyring is a signer service with one ECDSA key per identity, created on first use,
// so a test can sign as any identity and the code under test verifies with
// the real key.
type testKeyring struct {
	mu   sync.Mutex
	keys map[string]*ecdsa.PrivateKey
}

var testKeys = &testKeyring{keys: map[string]*ecdsa.PrivateKey{}}

func (k *testKeyring) key(id view.Identity) *ecdsa.PrivateKey {
	k.mu.Lock()
	defer k.mu.Unlock()
	if key, ok := k.keys[string(id)]; ok {
		return key
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	k.keys[string(id)] = key
	return key
}

func (k *testKeyring) GetSigner(id view.Identity) (cdriver.Signer, error) {
	return &testECDSASigner{key: k.key(id)}, nil
}

func (k *testKeyring) GetVerifier(id view.Identity) (cdriver.Verifier, error) {
	if len(id) == 0 {
		return nil, errors.New("empty identity")
	}
	if id.Equal(noVerifierID) {
		return nil, errors.New("cannot deserialize")
	}
	return &testECDSAVerifier{pub: &k.key(id).PublicKey}, nil
}

func (*testKeyring) GetSigningIdentity(view.Identity) (cdriver.SigningIdentity, error) {
	return nil, errors.New("not supported")
}
func (*testKeyring) IsMe(context.Context, view.Identity) bool         { return false }
func (*testKeyring) AreMe(context.Context, ...view.Identity) []string { return nil }

// sign returns x's proof for p, as the honest owner of x would make it.
func sign(t *testing.T, p ProofContext) []byte {
	t.Helper()
	signer, err := testKeys.GetSigner(p.Identity)
	require.NoError(t, err)
	proof, err := SignRecipientProof(signer, p)
	require.NoError(t, err)
	return proof
}

// testKMS loads ownNodeID as the node identity.
type testKMS struct{}

func (testKMS) Load(kmsdriver.ConfigProvider) (view.Identity, kmsdriver.Signer, kmsdriver.Verifier, error) {
	return ownNodeID, nil, nil, nil
}

type nopConfig struct{}

func (nopConfig) GetPath(string) string          { return "" }
func (nopConfig) GetStringSlice(string) []string { return nil }
func (nopConfig) TranslatePath(string) string    { return "" }

type nopSigService struct{}

func (nopSigService) RegisterSigner(context.Context, view.Identity, cdriver.Signer, cdriver.Verifier) error {
	return nil
}

// rejectionCounter counts security rejections by kind.
type rejectionCounter struct {
	disabled.Provider
	mu     sync.Mutex
	counts map[string]float64
}

func (r *rejectionCounter) NewCounter(metrics.CounterOpts) metrics.Counter {
	return &labelCounter{r: r}
}

type labelCounter struct {
	r    *rejectionCounter
	kind string
}

func (c *labelCounter) With(labelValues ...string) metrics.Counter {
	return &labelCounter{r: c.r, kind: labelValues[len(labelValues)-1]}
}

func (c *labelCounter) Add(delta float64) {
	c.r.mu.Lock()
	defer c.r.mu.Unlock()
	c.r.counts[c.kind] += delta
}

func (r *rejectionCounter) count(kind security.Kind) float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[string(kind)]
}

type recipientsEnv struct {
	ctx        *mockViewContext
	session    *mockSession
	store      *testBindingStore
	rejections *rejectionCounter
}

// newRecipientsEnv wires a view context whose session is session and whose
// services are nsp (absent when nil), a node identity provider for ownNodeID,
// a rejections counter, and an endpoint service backed by an in-memory binding
// store with a resolver "other" for otherNodeID. A session without an ID gets
// sessionID.
func newRecipientsEnv(t *testing.T, session *mockSession, nsp *fabric.NetworkServiceProvider) *recipientsEnv {
	t.Helper()
	if session.info.ID == "" {
		session.info.ID = sessionID
	}
	store := newTestBindingStore()
	es, err := endpoint.NewService(store)
	require.NoError(t, err)
	_, err = es.AddResolver("other", "", nil, nil, otherNodeID)
	require.NoError(t, err)
	idp, err := id.NewProvider(nopConfig{}, nopSigService{}, es, &kms.KMS{Driver: testKMS{}})
	require.NoError(t, err)
	rc := &rejectionCounter{counts: map[string]float64{}}
	rejections := security.NewRejections(rc)
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
			case reflect.TypeFor[*id.Provider]():
				return idp, nil
			case reflect.TypeFor[*security.Rejections]():
				return rejections, nil
			}
			return nil, errors.New("service missing")
		},
	}
	return &recipientsEnv{ctx: ctx, session: session, store: store, rejections: rc}
}

// testMembership is a channel membership whose configuration in force is the
// MSPs of mgr, of which those in idemix are of type Idemix. With a nil mgr the
// channel has no configuration in force.
type testMembership struct {
	fdriver.ChannelMembership
	mgr    fabricmsp.MSPManager
	idemix map[string]bool
}

func newTestMembership(t *testing.T, msps ...fabricmsp.MSP) *testMembership {
	t.Helper()
	mgr := fabricmsp.NewMSPManager()
	require.NoError(t, mgr.Setup(msps))
	m := &testMembership{mgr: mgr, idemix: map[string]bool{}}
	for _, msp := range msps {
		id, err := msp.GetIdentifier()
		require.NoError(t, err)
		m.idemix[id] = msp.GetType() == fabricmsp.IDEMIX
	}
	return m
}

func (m *testMembership) ConfigSequence() (uint64, error) {
	if m.mgr == nil {
		return 0, fdriver.ErrNotInitialized
	}
	return 1, nil
}

func (m *testMembership) IsIdemixMSP(mspID string) (bool, error) {
	if m.mgr == nil {
		return false, fdriver.ErrNotInitialized
	}
	return m.idemix[mspID], nil
}

func (m *testMembership) IsValid(identity view.Identity) error {
	id, err := m.mgr.DeserializeIdentity(identity)
	if err != nil {
		return err
	}
	return id.Validate()
}

func (m *testMembership) GetVerifier(identity view.Identity) (fdriver.Verifier, error) {
	return m.mgr.DeserializeIdentity(identity)
}

// nspWithMembership returns a network with default identity "me-id" whose
// channels all have membership m.
func nspWithMembership(m fdriver.ChannelMembership) *fabric.NetworkServiceProvider {
	return newTestFabricNetworkServiceProvider(view.Identity("me-id"), &mockDriverChannel{name: "ch", membership: m})
}

// recipientsNSP returns a network whose channel has no idemix MSP, so every
// recipient identity verifies with testKeys.
func recipientsNSP() *fabric.NetworkServiceProvider {
	return nspWithMembership(&testMembership{mgr: fabricmsp.NewMSPManager()})
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

// remoteProof returns the context a remote node signs over when it sends x to
// this node with tag: from otherNodeID on initiator sessions, from callerID on
// responder sessions.
func remoteProof(tag ProofTag, signerNode, x view.Identity) ProofContext {
	return ProofContext{Tag: tag, SessionID: sessionID, ContextID: "ctx", SignerNode: signerNode, VerifierNode: ownNodeID, Identity: x}
}

// provenReply is the honest reply of otherNodeID carrying x, for tag.
func provenReply(t *testing.T, tag ProofTag, x view.Identity) *view.Message {
	t.Helper()
	return okMsg(t, &RecipientData{Identity: x, Proof: sign(t, remoteProof(tag, otherNodeID, x))})
}

// provenRequest is callerID's honest exchange request carrying x.
func provenRequest(t *testing.T, x view.Identity) *view.Message {
	t.Helper()
	return okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: x, Proof: sign(t, remoteProof(TagExchangeRequest, callerID, x))}})
}

// requireOwnProof checks that data carries a valid proof by this node, toward
// verifierNode, for its identity.
func requireOwnProof(t *testing.T, tag ProofTag, verifierNode view.Identity, data *RecipientData) {
	t.Helper()
	v, err := testKeys.GetVerifier(data.Identity)
	require.NoError(t, err)
	p := ProofContext{Tag: tag, SessionID: sessionID, ContextID: "ctx", SignerNode: ownNodeID, VerifierNode: verifierNode, Identity: data.Identity}
	require.NoError(t, VerifyRecipientProof(v, p, data.Proof))
}

func TestExchangeRecipientIdentitiesViewCall(t *testing.T) {
	t.Parallel()

	reply := func(t *testing.T) *view.Message {
		t.Helper()
		return provenReply(t, TagExchangeReply, view.Identity("other-recipient"))
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
			requireOwnProof(t, TagExchangeRequest, otherNodeID, req.RecipientData)

			require.Equal(t, map[string]view.Identity{
				"other-recipient": otherNodeID,
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
		{name: "bind other error", failBind: otherNodeID, sent: 1, wantErr: "failed storing binding"},
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
		return provenRequest(t, view.Identity("other-recipient"))
	}
	caller := view.SessionInfo{Caller: callerID}

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
			requireOwnProof(t, TagExchangeReply, callerID, data)

			require.Equal(t, map[string]view.Identity{
				string(tc.me):     view.Identity("me"),
				"other-recipient": callerID,
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
		{name: "bind caller error", failBind: callerID, sent: 1, wantErr: "failed storing binding"},
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

	reply := provenReply(t, TagRequestReply, view.Identity("other-recipient"))
	env := newRecipientsEnv(t, &mockSession{recv: newRecv(reply)}, recipientsNSP())

	out, err := (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
	require.NoError(t, err)
	require.Equal(t, view.Identity("other-recipient"), out)
	require.Equal(t, map[string]view.Identity{"other-recipient": otherNodeID}, env.store.bindings)
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
			env := newRecipientsEnv(t, &mockSession{recv: tc.recv}, recipientsNSP())

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
		{name: "preset identity", identity: view.Identity("preset"), want: view.Identity("preset")},
		{name: "default identity", want: view.Identity("me-id")},
		{name: "fns error", nsp: failingNSP, wantErr: "fns failed"},
		{name: "send error", identity: view.Identity("preset"), sendErr: errors.New("send failed"), wantErr: "send failed"},
		{name: "bind error", identity: view.Identity("preset"), failBind: view.Identity("me"), wantErr: "failed storing bindings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			nsp := recipientsNSP()
			if tc.nsp != nil {
				nsp = tc.nsp()
			}
			session := &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(okMsg(t, &RecipientRequest{})), sendErr: tc.sendErr}
			env := newRecipientsEnv(t, session, nsp)
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
			requireOwnProof(t, TagRequestReply, callerID, data)
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

func TestRespondExchange_MissingProofRepliesError(t *testing.T) {
	t.Parallel()
	req := okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: view.Identity("x")}})
	env := newRecipientsEnv(t, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(req)}, recipientsNSP())

	_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
	require.ErrorIs(t, err, ErrRecipientProof)
	require.ErrorContains(t, err, "recipient proof: missing")
	require.Empty(t, env.session.sent, "no success reply")
	require.Equal(t, [][]byte{[]byte("recipient proof: missing")}, env.session.sentErrs)
	require.Empty(t, env.store.bindings)
	require.InDelta(t, 1, env.rejections.count(security.KindRecipientProof), 0)
}

func TestRespondExchange_ValidProofBinds(t *testing.T) {
	t.Parallel()
	env := newRecipientsEnv(t, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(provenRequest(t, view.Identity("x")))}, recipientsNSP())

	_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
	require.NoError(t, err)
	require.Len(t, env.session.sent, 1)
	data := &RecipientData{}
	require.NoError(t, data.FromBytes(env.session.sent[0]))
	requireOwnProof(t, TagExchangeReply, callerID, data)
	require.Equal(t, callerID, env.store.bindings["x"])
	require.Zero(t, env.rejections.count(security.KindRecipientProof))
}

// A refused proof must not be answered with this node's identity: the
// initiator would get a proven identity for an exchange the responder refused.
func TestRespondExchange_VerifiesBeforeReplying(t *testing.T) {
	t.Parallel()
	for name, p := range map[string]ProofContext{
		"wrong direction": remoteProof(TagExchangeReply, callerID, view.Identity("x")),
		"other session":   {Tag: TagExchangeRequest, SessionID: "other", ContextID: "ctx", SignerNode: callerID, VerifierNode: ownNodeID, Identity: view.Identity("x")},
		"other signer":    remoteProof(TagExchangeRequest, view.Identity("mallory"), view.Identity("x")),
		"other identity":  remoteProof(TagExchangeRequest, callerID, view.Identity("y")),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req := okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: view.Identity("x"), Proof: sign(t, p)}})
			env := newRecipientsEnv(t, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(req)}, recipientsNSP())

			_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
			require.ErrorIs(t, err, ErrRecipientProof)
			require.Empty(t, env.session.sent)
			require.Len(t, env.session.sentErrs, 1)
			require.Empty(t, env.store.bindings)
		})
	}
}

// Mallory relays a proof that x's owner made for mallory's node to carol: it
// names mallory as verifier, so carol refuses it.
func TestRespondExchange_RelayedProofRefused(t *testing.T) {
	t.Parallel()
	relayed := ProofContext{Tag: TagExchangeRequest, SessionID: sessionID, ContextID: "ctx", SignerNode: callerID, VerifierNode: view.Identity("mallory"), Identity: view.Identity("x")}
	req := okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: view.Identity("x"), Proof: sign(t, relayed)}})
	env := newRecipientsEnv(t, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(req)}, recipientsNSP())

	_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
	require.ErrorIs(t, err, ErrRecipientProof)
	require.Empty(t, env.store.bindings)
	require.InDelta(t, 1, env.rejections.count(security.KindRecipientProof), 0)
}

func TestRequestRecipient_UnprovenReplyRefused(t *testing.T) {
	t.Parallel()
	reply := okMsg(t, &RecipientData{Identity: view.Identity("x")})
	env := newRecipientsEnv(t, &mockSession{recv: newRecv(reply)}, recipientsNSP())

	_, err := (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
	require.ErrorIs(t, err, ErrRecipientProof)
	require.ErrorContains(t, err, "recipient proof: missing")
	require.Empty(t, env.store.bindings)
	require.InDelta(t, 1, env.rejections.count(security.KindRecipientProof), 0)
}

func TestExchange_UnprovenReplyRefused(t *testing.T) {
	t.Parallel()
	for name, reply := range map[string]*view.Message{
		"missing":      okMsg(t, &RecipientData{Identity: view.Identity("x")}),
		"request tag":  provenReply(t, TagRequestReply, view.Identity("x")),
		"other signer": okMsg(t, &RecipientData{Identity: view.Identity("x"), Proof: sign(t, remoteProof(TagExchangeReply, view.Identity("mallory"), view.Identity("x")))}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := newRecipientsEnv(t, &mockSession{recv: newRecv(reply)}, recipientsNSP())

			_, err := (&ExchangeRecipientIdentitiesView{Other: view.Identity("other")}).Call(env.ctx)
			require.ErrorIs(t, err, ErrRecipientProof)
			require.Empty(t, env.store.bindings, "neither the counterparty's nor this node's identity is bound")
		})
	}
}

func TestRecipient_ProvenBindRepairsPlantedRow(t *testing.T) {
	t.Parallel()
	env := newRecipientsEnv(t, &mockSession{recv: newRecv(provenReply(t, TagExchangeReply, view.Identity("x")))}, recipientsNSP())
	env.store.bindings["x"] = view.Identity("mallory")

	_, err := (&ExchangeRecipientIdentitiesView{Other: view.Identity("other")}).Call(env.ctx)
	require.NoError(t, err)
	require.Equal(t, otherNodeID, env.store.bindings["x"])
}

func TestCheckProof_Reasons(t *testing.T) {
	t.Parallel()
	fns, err := recipientsNSP().FabricNetworkService("")
	require.NoError(t, err)
	x := view.Identity("x")
	valid := remoteProof(TagExchangeRequest, callerID, x)
	noSession := valid
	noSession.SessionID = ""
	noVerifier := remoteProof(TagExchangeRequest, callerID, noVerifierID)
	for name, tc := range map[string]struct {
		p      ProofContext
		proof  []byte
		reason string
	}{
		"missing":            {p: valid, reason: reasonMissing},
		"incomplete context": {p: noSession, proof: sign(t, noSession), reason: reasonIncompleteContext},
		"no verifier":        {p: noVerifier, proof: sign(t, noVerifier), reason: reasonNoVerifier},
		"invalid signature":  {p: valid, proof: sign(t, remoteProof(TagExchangeReply, callerID, x)), reason: reasonInvalidSignature},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			reason, err := checkProof(fns, "", tc.p, tc.proof)
			require.ErrorIs(t, err, ErrRecipientProof)
			require.Equal(t, tc.reason, reason)
		})
	}
	reason, err := checkProof(fns, "", valid, sign(t, valid))
	require.NoError(t, err)
	require.Empty(t, reason)
}

// A counterparty whose identity this node cannot deserialize is refused, and
// nothing is bound.
func TestRespondExchange_NoVerifierRefused(t *testing.T) {
	t.Parallel()
	p := remoteProof(TagExchangeRequest, callerID, noVerifierID)
	req := okMsg(t, &ExchangeRecipientRequest{RecipientData: &RecipientData{Identity: noVerifierID, Proof: sign(t, p)}})
	env := newRecipientsEnv(t, &mockSession{info: view.SessionInfo{Caller: callerID}, recv: newRecv(req)}, recipientsNSP())

	_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(env.ctx)
	require.ErrorIs(t, err, ErrRecipientProof)
	require.ErrorContains(t, err, "recipient proof: no verifier")
	require.Empty(t, env.session.sent)
	require.Len(t, env.session.sentErrs, 1)
	require.Empty(t, env.store.bindings)
	require.InDelta(t, 1, env.rejections.count(security.KindRecipientProof), 0)
}

// Without a RemotePKID, the initiator takes the node identity from the
// resolver of the party it dialed: the proof must name otherNodeID, not the
// party, and the binding goes to otherNodeID.
func TestRequestRecipient_ResolverFallback(t *testing.T) {
	t.Parallel()
	x := view.Identity("x")

	env := newRecipientsEnv(t, &mockSession{recv: newRecv(provenReply(t, TagRequestReply, x))}, recipientsNSP())
	_, err := (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
	require.NoError(t, err)
	require.Equal(t, otherNodeID, env.store.bindings["x"])

	byParty := okMsg(t, &RecipientData{Identity: x, Proof: sign(t, remoteProof(TagRequestReply, view.Identity("other"), x))})
	env = newRecipientsEnv(t, &mockSession{recv: newRecv(byParty)}, recipientsNSP())
	_, err = (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(env.ctx)
	require.ErrorIs(t, err, ErrRecipientProof)
	require.Empty(t, env.store.bindings)

	env = newRecipientsEnv(t, &mockSession{recv: newRecv(provenReply(t, TagRequestReply, x))}, recipientsNSP())
	_, err = (&RequestRecipientIdentityView{Other: view.Identity("unknown")}).Call(env.ctx)
	require.ErrorContains(t, err, "failed resolving node identity of")
	require.Empty(t, env.session.sent, "nothing is sent to an unresolvable party")
	require.Empty(t, env.store.bindings)
}
