/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package p2p_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	viewmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
	p2pmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

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
	respCtx   *viewmock.Context
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
		responder: &viewmock.View{},
		respCtx:   &viewmock.Context{},
	}

	cl := &p2pmock.CommLayer{}
	masterSess := &viewmock.Session{}
	ch := make(chan *view.Message, 1)
	masterSess.ReceiveReturns(ch)
	cl.MasterSessionReturns(masterSess, nil)
	cl.NewResponderSessionReturns(&viewmock.Session{}, nil)

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
