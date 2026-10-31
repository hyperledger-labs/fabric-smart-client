/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	endpointmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
	p2pmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type viewManagerMock struct {
	HandleResponderCalled chan struct{}

	ExistResponderForCallerFunc func(caller string) (view.View, view.Identity, error)
	GetIdentityFunc             func(endpoint string, pkID []byte) (view.Identity, error)
	NewSessionContextFunc       func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, func(), error)
	DefaultIdentityFunc         func() view.Identity
}

func (m *viewManagerMock) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	if m.ExistResponderForCallerFunc != nil {
		return m.ExistResponderForCallerFunc(caller)
	}
	return &mock.View{}, nil, nil
}

func (m *viewManagerMock) GetIdentity(endpoint string, pkID []byte) (view.Identity, error) {
	if m.GetIdentityFunc != nil {
		return m.GetIdentityFunc(endpoint, pkID)
	}
	return view.Identity("caller"), nil
}

func (m *viewManagerMock) NewResponderContext(ctx context.Context, contextID string, session view.Session, me, _ view.Identity) (view.Context, func(), error) {
	if m.NewSessionContextFunc != nil {
		return m.NewSessionContextFunc(ctx, contextID, session, me)
	}
	return &mock.Context{}, func() {}, nil
}

func (m *viewManagerMock) DefaultIdentity() view.Identity {
	if m.DefaultIdentityFunc != nil {
		return m.DefaultIdentityFunc()
	}
	return view.Identity("me")
}

func TestService(t *testing.T) {
	t.Parallel()
	vm := &viewManagerMock{HandleResponderCalled: make(chan struct{}, 10)}
	cl := &p2pmock.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner(), nil)
	ctx := t.Context()

	err := service.Start(ctx)
	require.NoError(t, err)

	// Send a message
	msg := &view.Message{
		ContextID:    "ctx1",
		SessionID:    "sess1",
		Caller:       "caller1",
		FromEndpoint: "endpoint1",
		FromPKID:     []byte("pkid1"),
		Ctx:          ctx,
	}

	vm.ExistResponderForCallerFunc = func(caller string) (view.View, view.Identity, error) {
		require.Equal(t, "caller1", caller)
		return &mock.View{}, nil, nil
	}
	released := make(chan struct{}, 1)
	vm.NewSessionContextFunc = func(_ context.Context, contextID string, _ view.Session, _ view.Identity) (view.Context, func(), error) {
		require.Equal(t, "ctx1", contextID)
		vm.HandleResponderCalled <- struct{}{}
		return &mock.Context{}, func() { released <- struct{}{} }, nil
	}

	ch <- msg

	select {
	case <-vm.HandleResponderCalled:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("NewSessionContext was not called")
	}
	select {
	case <-released:
		// respond releases the context when the responder returns
	case <-time.After(5 * time.Second):
		t.Fatal("the responder context was not released")
	}
}

// The responder's caller is the peer ID the transport verified. A peer connecting from
// another node's registered address gets its own identity, not that node's.
func TestService_CallerIsVerifiedPeerID(t *testing.T) {
	t.Parallel()
	es, err := endpoint.NewService(&endpointmock.BindingStore{})
	require.NoError(t, err)
	victimAddr := "127.0.0.1:20001"
	_, err = es.AddResolver("victim", "org1", map[string]string{string(endpoint.P2PPort): victimAddr}, nil, []byte("victim_id"))
	require.NoError(t, err)
	_, err = es.AddResolver("attacker", "org2", map[string]string{string(endpoint.P2PPort): "127.0.0.1:20002"}, nil, []byte("attacker_id"))
	require.NoError(t, err)

	vm := &viewManagerMock{HandleResponderCalled: make(chan struct{}, 10)}
	cl := &p2pmock.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)
	vm.NewSessionContextFunc = func(context.Context, string, view.Session, view.Identity) (view.Context, func(), error) {
		vm.HandleResponderCalled <- struct{}{}
		return &mock.Context{}, func() {}, nil
	}
	require.NoError(t, p2p.NewService(vm, vm, cl, es, p2p.NewDefaultRunner(), nil).Start(t.Context()))

	for _, from := range []string{victimAddr, "127.0.0.1:54321"} {
		ch <- &view.Message{ContextID: "ctx", SessionID: from, Caller: "caller", FromEndpoint: from, FromPKID: []byte("attacker_id"), Ctx: t.Context()}
		select {
		case <-vm.HandleResponderCalled:
		case <-time.After(5 * time.Second):
			t.Fatal("NewResponderContext was not called")
		}
		remote, _ := cl.NewResponderSessionArgsForCall(cl.NewResponderSessionCallCount() - 1)
		require.Equal(t, []byte("attacker_id"), remote, "message from [%s]", from)
	}
}

func TestService_MasterSessionError(t *testing.T) {
	t.Parallel()
	vm := &viewManagerMock{}
	cl := &p2pmock.CommLayer{}
	cl.MasterSessionReturns(nil, errors.New("master session error"))

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner(), nil)
	err := service.Start(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed getting master session")
}

// TestService_PanicIsReturnedToRemoteCaller demonstrates that when a responder view
// panics (e.g. endorser.Transaction.Namespaces() panics with
// `panic(errors.Wrap(err, "failed getting rw set").Error())` when the local RWSet cannot be
// read), Service.respond sends the resulting error back to the remote caller via
// Session.SendError, exactly as it would for any other responder-view error.
func TestService_PanicIsReturnedToRemoteCaller(t *testing.T) {
	t.Parallel()

	sensitivePanicText := "failed getting rw set: could not open /var/lib/fabric/kvs/vault.db: permission denied"

	panicView := &mock.View{}
	panicView.CallStub = func(view.Context) (any, error) {
		panic(sensitivePanicText)
	}

	vm := &viewManagerMock{
		HandleResponderCalled: make(chan struct{}, 10),
	}
	vm.ExistResponderForCallerFunc = func(_ string) (view.View, view.Identity, error) {
		return panicView, nil, nil
	}

	respSession := &mock.Session{}
	respSession.SendErrorReturns(nil)

	respCtx := &mock.Context{}
	respCtx.SessionReturns(respSession)
	respCtx.ContextReturns(t.Context())
	// The default Runner (p2p.NewDefaultRunner) just delegates to viewCtx.RunView(responder).
	// The real implementation (viewpkg.RunViewNow) recovers any panic raised by the
	// responder's Call and turns it into an error (wrapping ErrViewExecutionFailed with the
	// panic value) rather than letting it propagate - mirror that here.
	respCtx.RunViewStub = func(v view.View, _ ...view.RunViewOption) (res any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("caught panic: %v", r)
			}
		}()
		return v.Call(respCtx)
	}
	vm.NewSessionContextFunc = func(_ context.Context, _ string, _ view.Session, _ view.Identity) (view.Context, func(), error) {
		vm.HandleResponderCalled <- struct{}{}
		return respCtx, func() {}, nil
	}

	cl := &p2pmock.CommLayer{}
	masterSess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(masterSess, nil)
	masterSess.ReceiveReturns(ch)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner(), nil)
	ctx := t.Context()

	err := service.Start(ctx)
	require.NoError(t, err)

	ch <- &view.Message{
		ContextID:    "ctx1",
		SessionID:    "sess1",
		Caller:       "caller1",
		FromEndpoint: "endpoint1",
		FromPKID:     []byte("pkid1"),
		Ctx:          ctx,
	}

	select {
	case <-vm.HandleResponderCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("NewResponderContext was not called")
	}

	// respond() runs the panicking view asynchronously (handleMessage is invoked via
	// `go s.handleMessage(msg)`); poll briefly for the resulting SendError call.
	require.Eventually(t, func() bool {
		return respSession.SendErrorCallCount() > 0
	}, 5*time.Second, 10*time.Millisecond, "SendError was never called with the panic's error text")

	sentCtx, sentPayload := respSession.SendErrorArgsForCall(0)
	require.NotNil(t, sentCtx)
	require.NoError(t, sentCtx.Err())
	require.Contains(t, string(sentPayload), sensitivePanicText)
}

func TestService_HandleResponderError(t *testing.T) {
	t.Parallel()
	vm := &viewManagerMock{
		HandleResponderCalled: make(chan struct{}, 10),
	}
	vm.NewSessionContextFunc = func(_ context.Context, _ string, _ view.Session, _ view.Identity) (view.Context, func(), error) {
		vm.HandleResponderCalled <- struct{}{}
		return &mock.Context{}, func() {}, nil
	}

	cl := &p2pmock.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner(), nil)
	ctx := t.Context()

	err := service.Start(ctx)
	require.NoError(t, err)

	// Send a message
	msg := &view.Message{
		ContextID: "ctx1",
		Caller:    "caller",
		Ctx:       ctx,
	}
	ch <- msg

	select {
	case <-vm.HandleResponderCalled:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("NewSessionContext was not called")
	}
}

// TestService_ClosesSessionWithoutContext checks that a responder session no context accepts is
// closed, since only Context.Dispose closes sessions otherwise.
func TestService_ClosesSessionWithoutContext(t *testing.T) {
	t.Parallel()
	vm := &viewManagerMock{}
	vm.NewSessionContextFunc = func(context.Context, string, view.Session, view.Identity) (view.Context, func(), error) {
		return nil, nil, errors.New("context [ctx1] is not a responder context")
	}
	cl := &p2pmock.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 1)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)
	responderSession := &mock.Session{}
	cl.NewResponderSessionReturns(responderSession, nil)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner(), nil)
	ctx := t.Context()
	require.NoError(t, service.Start(ctx))

	ch <- &view.Message{ContextID: "ctx1", SessionID: "sess1", Caller: "caller", FromPKID: []byte("pkid1"), Ctx: ctx}

	require.Eventually(t, func() bool { return responderSession.CloseCallCount() == 1 }, 5*time.Second, 10*time.Millisecond)
	require.Zero(t, sess.CloseCallCount(), "the master session stays open")
}

// blockingRunner blocks in RunView until the view context it is given is cancelled,
// simulating a long-running responder view that must observe shutdown.
type blockingRunner struct{ entered chan struct{} }

func (r *blockingRunner) RunView(viewCtx view.Context, _ view.View) (any, error) {
	close(r.entered)
	<-viewCtx.Context().Done()
	return nil, nil
}

// leakTestDeps is a minimal stand-in for ViewManager, IdentityProvider and EndpointService.
// NewResponderContext hands back respCtx with its Context() bound to whatever ctx the
// production code actually passes in, so the test observes exactly what Service threads
// through, rather than assuming it.
type leakTestDeps struct {
	responder view.View
	respCtx   *mock.Context
}

func (d *leakTestDeps) ExistResponderForCaller(_ string) (view.View, view.Identity, error) {
	return d.responder, nil, nil
}

func (d *leakTestDeps) NewResponderContext(ctx context.Context, _ string, _ view.Session, _, _ view.Identity) (view.Context, func(), error) {
	d.respCtx.ContextReturns(ctx)
	return d.respCtx, func() {}, nil
}

func (*leakTestDeps) DefaultIdentity() view.Identity { return view.Identity("me") }

func (*leakTestDeps) GetIdentity(_ string, _ []byte) (view.Identity, error) {
	return view.Identity("caller"), nil
}

// TestService_Start_DrainsHandlersOnShutdown verifies that Start's read loop does not
// return until every in-flight handleMessage goroutine it spawned has finished.
//
// msg.Ctx is deliberately set to context.Background(), a context that is never cancelled
// during this test. That means the *only* way blockingRunner can ever unblock is if the
// production code parents the responder's view.Context on the ctx passed to Start (which
// this test does cancel), rather than on msg.Ctx. This makes the test a reliable, non-racy
// signal instead of a timing race between two goroutines woken by the same channel close:
//   - against the unfixed Start (no WaitGroup, no ctx threading), the handler blocks on
//     msg.Ctx forever and goleak reliably reports the leaked goroutine within its retry
//     window;
//   - against the fix, the handler observes Start's ctx and Start's wg.Wait() drains it
//     before returning, so goleak reliably reports no leak.
func TestService_Start_DrainsHandlersOnShutdown(t *testing.T) { //nolint:paralleltest // uses goleak.VerifyNone; must run serially
	defer goleak.VerifyNone(t)

	entered := make(chan struct{})
	runner := &blockingRunner{entered: entered}

	deps := &leakTestDeps{
		responder: &mock.View{},
		respCtx:   &mock.Context{},
	}

	cl := &p2pmock.CommLayer{}
	masterSess := &mock.Session{}
	ch := make(chan *view.Message, 1)
	masterSess.ReceiveReturns(ch)
	cl.MasterSessionReturns(masterSess, nil)
	cl.NewResponderSessionReturns(&mock.Session{}, nil)

	service := p2p.NewService(deps, deps, cl, deps, runner, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := service.Start(ctx)
	require.NoError(t, err)

	// Deliver exactly one message through session.Receive().
	ch <- &view.Message{
		ContextID:    "ctx1",
		SessionID:    "sess1",
		Caller:       "caller1",
		FromEndpoint: "endpoint1",
		FromPKID:     []byte("pkid1"),
		Ctx:          context.Background(), // never cancelled - see doc comment above
	}

	// Wait until the handler is confirmed in-flight (blocked inside RunView).
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("handler was never invoked")
	}

	// Trigger shutdown.
	cancel()

	// Belt-and-braces settle in addition to goleak's own retry/backoff (up to ~400ms by
	// default) so the fixed Start's drain has time to complete before we return.
	time.Sleep(20 * time.Millisecond)
}

// TestService_Start_StopsOnClosedMasterSession checks that Start's loop returns once the
// master session's Receive channel is closed, instead of reading nil messages from it and
// dispatching them to handlers. ctx is never cancelled, so the loop can only exit by
// observing the closed channel; goleak reports it otherwise.
func TestService_Start_StopsOnClosedMasterSession(t *testing.T) { //nolint:paralleltest // uses goleak.VerifyNone; must run serially
	defer goleak.VerifyNone(t)

	ch := make(chan *view.Message)
	close(ch)
	master := &mock.Session{}
	master.ReceiveReturns(ch)
	cl := &p2pmock.CommLayer{}
	cl.MasterSessionReturns(master, nil)
	vm := &viewManagerMock{ExistResponderForCallerFunc: func(string) (view.View, view.Identity, error) {
		t.Error("no message must be dispatched from a closed master session")
		return nil, nil, errors.New("unexpected")
	}}

	require.NoError(t, p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner(), nil).Start(context.Background()))
}

type limitConfig int

func (limitConfig) IsSet(key string) bool { return key == p2p.MaxRespondersPerPeerKey }

func (c limitConfig) GetInt(string) int { return int(c) }

// releasableRunner blocks in RunView until the test releases that view context's ID or the
// view context is done, signalling entered on every invocation.
type releasableRunner struct {
	entered  chan struct{}
	releases sync.Map // view context ID -> chan struct{}
}

// release returns the channel that, once closed, releases the responder running in the
// view context with the given ID.
func (r *releasableRunner) release(contextID string) chan struct{} {
	c, _ := r.releases.LoadOrStore(contextID, make(chan struct{}))
	return c.(chan struct{})
}

func (r *releasableRunner) RunView(viewCtx view.Context, _ view.View) (any, error) {
	r.entered <- struct{}{}
	select {
	case <-r.release(viewCtx.ID()):
	case <-viewCtx.Context().Done():
	}
	return nil, nil
}

// freshContextDeps hands back a new view context, bound to the ctx Service passes in, on
// every NewResponderContext call.
type freshContextDeps struct{ leakTestDeps }

func (*freshContextDeps) NewResponderContext(ctx context.Context, contextID string, _ view.Session, _, _ view.Identity) (view.Context, func(), error) {
	c := &mock.Context{}
	c.ContextReturns(ctx)
	c.IDReturns(contextID)
	return c, func() {}, nil
}

// limitTest wires a Service with the given per-peer limit to a fake master session.
type limitTest struct {
	t      *testing.T
	runner *releasableRunner
	cl     *p2pmock.CommLayer
	ch     chan *view.Message
}

func newLimitTest(t *testing.T, cfg p2p.ConfigService, buffer int) *limitTest {
	t.Helper()
	lt := &limitTest{
		t:      t,
		runner: &releasableRunner{entered: make(chan struct{}, buffer)},
		cl:     &p2pmock.CommLayer{},
		ch:     make(chan *view.Message, buffer),
	}
	masterSess := &mock.Session{}
	masterSess.ReceiveReturns(lt.ch)
	lt.cl.MasterSessionReturns(masterSess, nil)
	lt.cl.NewResponderSessionReturns(&mock.Session{}, nil)
	deps := &freshContextDeps{leakTestDeps{responder: &mock.View{}}}
	require.NoError(t, p2p.NewService(deps, deps, lt.cl, deps, lt.runner, cfg).Start(t.Context()))
	return lt
}

// send delivers a first message with context "ctx-<id>" on session sessionID from peer.
func (lt *limitTest) send(id, sessionID, peer string) {
	lt.ch <- &view.Message{
		ContextID:    "ctx-" + id,
		SessionID:    sessionID,
		Caller:       "caller",
		FromEndpoint: peer,
		FromPKID:     []byte(peer),
		Ctx:          context.Background(),
	}
}

func (lt *limitTest) wait(c <-chan struct{}, what string) {
	lt.t.Helper()
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		lt.t.Fatalf("timed out waiting for %s", what)
	}
}

// TestService_LimitsRespondersPerPeer verifies that one peer cannot run more than the
// configured number of responders: the first message over the limit is rejected with an
// error reply, messages beyond the bound on pending rejections are dropped, other peers are
// unaffected, a pending rejection does not use up a responder slot, and a finished
// responder frees its slot.
func TestService_LimitsRespondersPerPeer(t *testing.T) {
	t.Parallel()
	lt := newLimitTest(t, limitConfig(1), 10)

	rejectSending := make(chan struct{}, 10)
	unblockReject := make(chan struct{})
	lt.cl.ReplyErrorStub = func(context.Context, *view.Message, []byte) error {
		select { // non-blocking: later rejections must not wedge the test
		case rejectSending <- struct{}{}:
		default:
		}
		<-unblockReject
		return nil
	}

	lt.send("a1", "sess-a1", "peerA")
	lt.wait(lt.runner.entered, "a1 to run")
	lt.send("a2", "sess-a2", "peerA")
	lt.wait(rejectSending, "a2 to be rejected")
	lt.send("a3", "sess-a3", "peerA") // over both bounds: dropped
	lt.send("b1", "sess-b1", "peerB")
	lt.wait(lt.runner.entered, "b1 to run")

	// Start dispatches sequentially, so a3 was handled before b1. Had a3 been run or
	// rejected instead of dropped, it would show up on entered or rejectSending.
	require.Never(t, func() bool { return len(lt.runner.entered) > 0 || len(rejectSending) > 0 }, 100*time.Millisecond, 5*time.Millisecond)
	require.Equal(t, 2, lt.cl.NewResponderSessionCallCount(), "sessions for a1 and b1")
	require.Equal(t, 1, lt.cl.ReplyErrorCallCount(), "rejection of a2")
	_, rejected, payload := lt.cl.ReplyErrorArgsForCall(0)
	require.Equal(t, "sess-a2", rejected.SessionID)
	require.Equal(t, "busy: too many concurrent responders for this peer", string(payload))

	// Free a1's slot while a2's rejection is still pending: peerA can run a responder again
	// once the slot is released. Attempts before that are dropped, as the rejection bound
	// is still full.
	close(lt.runner.release("ctx-a1"))
	i := 0
	require.Eventually(t, func() bool {
		i++
		lt.send(fmt.Sprintf("a4-%d", i), fmt.Sprintf("sess-a4-%d", i), "peerA")
		select {
		case <-lt.runner.entered:
			return true
		case <-time.After(20 * time.Millisecond):
			return false
		}
	}, 5*time.Second, time.Millisecond)

	close(unblockReject)
}

func TestNewService_MaxRespondersPerPeerConfig(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		cfg  p2p.ConfigService
		want int
	}{
		"unset":    {nil, p2p.DefaultMaxRespondersPerPeer},
		"zero":     {limitConfig(0), p2p.DefaultMaxRespondersPerPeer},
		"negative": {limitConfig(-1), p2p.DefaultMaxRespondersPerPeer},
		"set":      {limitConfig(3), 3},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lt := newLimitTest(t, tc.cfg, tc.want+1)
			for i := range tc.want + 1 {
				lt.send(fmt.Sprint(i), fmt.Sprint(i), "peer")
			}
			require.Eventually(t, func() bool { return len(lt.runner.entered) == tc.want }, 5*time.Second, time.Millisecond)
			require.Never(t, func() bool { return len(lt.runner.entered) > tc.want }, 50*time.Millisecond, 5*time.Millisecond)
		})
	}
}

// countingCounter counts the additions to a metrics.Counter per label values.
type countingCounter struct {
	mu     sync.Mutex
	counts map[string]float64
}

func newCountingCounter() *countingCounter { return &countingCounter{counts: map[string]float64{}} }

func (c *countingCounter) With(labelValues ...string) metrics.Counter {
	return labeledCounter{c, strings.Join(labelValues, "=")}
}

func (*countingCounter) Add(float64) { panic("rejections are always labeled") }

func (c *countingCounter) count(labels string) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[labels]
}

type labeledCounter struct {
	*countingCounter
	labels string
}

func (l labeledCounter) Add(delta float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[l.labels] += delta
}

// metricsProvider hands out counter for the rejections counter and checks its name.
type metricsProvider struct {
	metrics.Provider
	t       *testing.T
	counter *countingCounter
}

func (p *metricsProvider) NewCounter(o metrics.CounterOpts) metrics.Counter {
	require.Equal(p.t, "rejections", o.Name)
	require.Equal(p.t, []string{"reason"}, o.LabelNames)
	return p.counter
}

// unknownViewDeps hosts a responder only for the caller "hosted".
type unknownViewDeps struct{ freshContextDeps }

func (d *unknownViewDeps) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	if caller != "hosted" {
		return nil, nil, errors.New("no view found initiatable by [" + caller + "]")
	}
	return d.responder, nil, nil
}

// rejectTest wires a Service whose only responder answers the caller "hosted", with the
// given per-peer limit and a rejections counter.
type rejectTest struct {
	*limitTest
	counter *countingCounter
	service *p2p.Service
}

func newRejectTest(t *testing.T, limit int, opts ...p2p.Option) *rejectTest {
	t.Helper()
	rt := &rejectTest{
		limitTest: &limitTest{
			t:      t,
			runner: &releasableRunner{entered: make(chan struct{}, 10)},
			cl:     &p2pmock.CommLayer{},
			ch:     make(chan *view.Message, 10),
		},
		counter: newCountingCounter(),
	}
	masterSess := &mock.Session{}
	masterSess.ReceiveReturns(rt.ch)
	rt.cl.MasterSessionReturns(masterSess, nil)
	rt.cl.NewResponderSessionReturns(&mock.Session{}, nil)
	deps := &unknownViewDeps{freshContextDeps{leakTestDeps{responder: &mock.View{}}}}
	opts = append([]p2p.Option{p2p.WithMetrics(&metricsProvider{t: t, counter: rt.counter})}, opts...)
	rt.service = p2p.NewService(deps, deps, rt.cl, deps, rt.runner, limitConfig(limit), opts...)
	require.NoError(t, rt.service.Start(t.Context()))
	return rt
}

func (rt *rejectTest) sendMsg(msg *view.Message) {
	if msg.Ctx == nil {
		msg.Ctx = context.Background()
	}
	if msg.FromPKID == nil {
		msg.FromPKID = []byte("peer")
	}
	rt.ch <- msg
}

// TestService_UnknownViewGetsErrorReply checks that a first message for a view with no
// responder on this node gets the error "unknown_view: <caller>" instead of only a log line,
// so that the initiator fails at once, and that no responder session is created for it.
func TestService_UnknownViewGetsErrorReply(t *testing.T) {
	t.Parallel()
	rt := newRejectTest(t, 1)

	rt.sendMsg(&view.Message{ContextID: "ctx", SessionID: "sess", Caller: "github.com/acme/app/Unhosted"})

	require.Eventually(t, func() bool { return rt.cl.ReplyErrorCallCount() == 1 }, 5*time.Second, 5*time.Millisecond)
	_, msg, payload := rt.cl.ReplyErrorArgsForCall(0)
	require.Equal(t, "sess", msg.SessionID)
	require.Equal(t, "unknown_view: github.com/acme/app/Unhosted", string(payload))
	require.Zero(t, rt.cl.NewResponderSessionCallCount())
	require.Eventually(t, func() bool { return rt.counter.count("reason=unknown_view") == 1 }, 5*time.Second, 5*time.Millisecond)
}

// TestService_UnknownViewTakesRejectionSlot checks that unknown-view rejections share the
// per-peer bound on pending rejections, and do not take a responder slot.
func TestService_UnknownViewTakesRejectionSlot(t *testing.T) {
	t.Parallel()
	rt := newRejectTest(t, 1)
	sending := make(chan struct{}, 10)
	unblock := make(chan struct{})
	rt.cl.ReplyErrorStub = func(context.Context, *view.Message, []byte) error {
		sending <- struct{}{}
		<-unblock
		return nil
	}

	rt.sendMsg(&view.Message{ContextID: "u1", SessionID: "u1", Caller: "unhosted"})
	rt.wait(sending, "u1 to be rejected")
	rt.sendMsg(&view.Message{ContextID: "u2", SessionID: "u2", Caller: "unhosted"}) // rejection bound full: dropped
	rt.sendMsg(&view.Message{ContextID: "h1", SessionID: "h1", Caller: "hosted"})
	rt.wait(rt.runner.entered, "h1 to run")

	require.Never(t, func() bool { return len(sending) > 0 }, 50*time.Millisecond, 5*time.Millisecond)
	require.Equal(t, 1, rt.cl.ReplyErrorCallCount())
	close(unblock)
	close(rt.runner.release("h1"))
}

// TestService_BusyRejectionFreesItsSlot checks that the busy reply reaches the initiator,
// that its rejection slot is freed once sent, and that the responder limit still holds.
func TestService_BusyRejectionFreesItsSlot(t *testing.T) {
	t.Parallel()
	rt := newRejectTest(t, 1)

	rt.sendMsg(&view.Message{ContextID: "a1", SessionID: "a1", Caller: "hosted"})
	rt.wait(rt.runner.entered, "a1 to run")
	for i := range 3 { // each busy rejection must free its slot, or the later ones are dropped
		rt.sendMsg(&view.Message{ContextID: fmt.Sprint("b", i), SessionID: fmt.Sprint("b", i), Caller: "hosted"})
		require.Eventually(t, func() bool { return rt.cl.ReplyErrorCallCount() == i+1 }, 5*time.Second, 5*time.Millisecond)
	}
	for i := range 3 {
		_, _, payload := rt.cl.ReplyErrorArgsForCall(i)
		require.Equal(t, "busy: too many concurrent responders for this peer", string(payload))
	}
	require.Empty(t, rt.runner.entered, "the responder limit holds")
	require.Equal(t, 1, rt.cl.NewResponderSessionCallCount())
	require.Eventually(t, func() bool { return rt.counter.count("reason=busy") == 3 }, 5*time.Second, 5*time.Millisecond)
	close(rt.runner.release("a1"))
}

// TestService_DropsUnanswerableMessages checks that a first message with status ERROR or an
// empty caller neither starts a responder nor gets a reply, whether or not its view is hosted.
func TestService_DropsUnanswerableMessages(t *testing.T) {
	t.Parallel()
	for name, msg := range map[string]*view.Message{
		"error status, hosted view":  {Status: view.ERROR, Caller: "hosted"},
		"error status, unknown view": {Status: view.ERROR, Caller: "unhosted"},
		"error status, empty caller": {Status: view.ERROR},
		"ok status, empty caller":    {Status: view.OK},
		"zero status, empty caller":  {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rt := newRejectTest(t, 1)
			msg.ContextID, msg.SessionID = "ctx", "sess"
			rt.sendMsg(msg)
			// A hosted message sent afterwards runs, so the dropped one was already handled.
			rt.sendMsg(&view.Message{ContextID: "h", SessionID: "h", Caller: "hosted", FromPKID: []byte("other")})
			rt.wait(rt.runner.entered, "the hosted message to run")
			// A responder started for the dropped message would show up on entered too.
			require.Never(t, func() bool { return len(rt.runner.entered) > 0 }, 50*time.Millisecond, 5*time.Millisecond)
			require.Zero(t, rt.cl.ReplyErrorCallCount())
			require.Equal(t, 1, rt.cl.NewResponderSessionCallCount())
			close(rt.runner.release("h"))
		})
	}
}

// TestService_RejectionsDoNotLoop wires two services whose comm layers deliver each other's
// error replies to the other's master session, as the transport does: A rejects B's message
// for an unknown view once, and B never answers the rejection.
func TestService_RejectionsDoNotLoop(t *testing.T) {
	t.Parallel()
	a, b := newRejectTest(t, 1), newRejectTest(t, 1)
	deliver := func(to *rejectTest, from string) func(context.Context, *view.Message, []byte) error {
		return func(_ context.Context, msg *view.Message, payload []byte) error {
			// A ViewPacket from ReplyError: status ERROR, empty caller.
			to.sendMsg(&view.Message{ContextID: msg.ContextID, SessionID: msg.SessionID, Status: view.ERROR, Payload: payload, FromPKID: []byte(from)})
			return nil
		}
	}
	a.cl.ReplyErrorStub = deliver(b, "a")
	b.cl.ReplyErrorStub = deliver(a, "b")

	a.sendMsg(&view.Message{ContextID: "ctx", SessionID: "sess", Caller: "unhosted", FromPKID: []byte("b")})

	require.Eventually(t, func() bool { return a.cl.ReplyErrorCallCount() == 1 }, 5*time.Second, 5*time.Millisecond)
	require.Never(t, func() bool { return b.cl.ReplyErrorCallCount() > 0 || a.cl.ReplyErrorCallCount() > 1 }, 100*time.Millisecond, 5*time.Millisecond)
}

func TestSanitizeDetail(t *testing.T) {
	t.Parallel()
	require.Equal(t, "github.com/acme/app/View", p2p.SanitizeDetail("github.com/acme/app/View"))
	require.Equal(t, "a?b?c?", p2p.SanitizeDetail("a\nb\x1bc\u00e9"))
	require.Equal(t, strings.Repeat("x", 256), p2p.SanitizeDetail(strings.Repeat("x", 300)))
	require.Empty(t, p2p.SanitizeDetail(""))
}
