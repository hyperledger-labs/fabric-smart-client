/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type viewManagerMock struct {
	HandleResponderCalled chan struct{}

	ExistResponderForCallerFunc func(caller string) (view.View, view.Identity, error)
	GetIdentityFunc             func(endpoint string, pkID []byte) (view.Identity, error)
	NewSessionContextFunc       func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error)
	DeleteContextFunc           func(contextID string)
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

func (m *viewManagerMock) NewResponderContext(ctx context.Context, contextID string, session view.Session, me, remote view.Identity) (view.Context, bool, error) {
	if m.NewSessionContextFunc != nil {
		return m.NewSessionContextFunc(ctx, contextID, session, me)
	}
	return &mock.Context{}, true, nil
}

func (m *viewManagerMock) DeleteContext(contextID string) {
	if m.DeleteContextFunc != nil {
		m.DeleteContextFunc(contextID)
	}
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
	cl := &mock2.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner())
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
	vm.NewSessionContextFunc = func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
		require.Equal(t, "ctx1", contextID)
		vm.HandleResponderCalled <- struct{}{}
		return &mock.Context{}, true, nil
	}

	ch <- msg

	select {
	case <-vm.HandleResponderCalled:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("NewSessionContext was not called")
	}
}

func TestService_MasterSessionError(t *testing.T) {
	t.Parallel()
	vm := &viewManagerMock{}
	cl := &mock2.CommLayer{}
	cl.MasterSessionReturns(nil, errors.New("master session error"))

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner())
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
	vm.ExistResponderForCallerFunc = func(caller string) (view.View, view.Identity, error) {
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
	vm.NewSessionContextFunc = func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
		vm.HandleResponderCalled <- struct{}{}
		return respCtx, true, nil
	}

	cl := &mock2.CommLayer{}
	masterSess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(masterSess, nil)
	masterSess.ReceiveReturns(ch)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner())
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
	vm.NewSessionContextFunc = func(ctx context.Context, contextID string, session view.Session, party view.Identity) (view.Context, bool, error) {
		vm.HandleResponderCalled <- struct{}{}
		return &mock.Context{}, true, nil
	}

	cl := &mock2.CommLayer{}
	sess := &mock.Session{}
	ch := make(chan *view.Message, 10)
	cl.MasterSessionReturns(sess, nil)
	sess.ReceiveReturns(ch)

	service := p2p.NewService(vm, vm, cl, vm, p2p.NewDefaultRunner())
	ctx := t.Context()

	err := service.Start(ctx)
	require.NoError(t, err)

	// Send a message
	msg := &view.Message{
		ContextID: "ctx1",
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

func (d *leakTestDeps) ExistResponderForCaller(caller string) (view.View, view.Identity, error) {
	return d.responder, nil, nil
}

func (d *leakTestDeps) NewResponderContext(ctx context.Context, contextID string, session view.Session, me, remote view.Identity) (view.Context, bool, error) {
	d.respCtx.ContextReturns(ctx)
	return d.respCtx, true, nil
}

func (d *leakTestDeps) DeleteContext(contextID string) {}

func (d *leakTestDeps) DefaultIdentity() view.Identity { return view.Identity("me") }

func (d *leakTestDeps) GetIdentity(endpoint string, pkID []byte) (view.Identity, error) {
	return view.Identity("caller"), nil
}

// TestService_Start_DrainsHandlersOnShutdown verifies that Start's read loop does not
// return until every in-flight handleMessage goroutine it spawned has finished (Issue #7).
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

	cl := &mock2.CommLayer{}
	masterSess := &mock.Session{}
	ch := make(chan *view.Message, 1)
	masterSess.ReceiveReturns(ch)
	cl.MasterSessionReturns(masterSess, nil)
	cl.NewResponderSessionReturns(&mock.Session{}, nil)

	service := p2p.NewService(deps, deps, cl, deps, runner)

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
