/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/goleak"

	viewpkg "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestRunView_CancelStopsAsyncRun asserts that cancelling the context passed
// to the package-level RunView causes the goroutine running the view
// asynchronously to observe cancellation and stop.
func TestRunView_CancelStopsAsyncRun(t *testing.T) { //nolint:paralleltest // uses goleak.VerifyNone; must run serially
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})

	fake := &mock.Context{}
	fake.RunViewStub = func(v view.View, opts ...view.RunViewOption) (any, error) {
		options, err := view.CompileRunViewOptions(opts...)
		if err != nil {
			t.Errorf("failed compiling options: %v", err)
			return nil, err
		}
		if options.Ctx == nil {
			t.Error("expected the go context to be present in opts")
			return nil, nil
		}
		<-options.Ctx.Done()
		close(done)
		return nil, nil
	}

	viewpkg.RunView(ctx, fake, nil)

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunView goroutine did not observe context cancellation")
	}
}

// TestRunView_PreservesChildContextOptions guards against reintroducing
// view.WithContext here: that option also sets SameContext, which makes
// RunViewNow reuse the caller's view context and discard options.Session. That
// silently broke the mock pingpong integration test, whose responder view is
// started as RunView(ctx, c, v, view.AsResponder(right)) and then panicked on a
// nil viewCtx.Session().
func TestRunView_PreservesChildContextOptions(t *testing.T) { //nolint:paralleltest // uses goleak.VerifyNone; must run serially
	defer goleak.VerifyNone(t)

	ctx := t.Context()

	session := &mock.Session{}
	compiled := make(chan *view.RunViewOptions, 1)

	fake := &mock.Context{}
	fake.RunViewStub = func(v view.View, opts ...view.RunViewOption) (any, error) {
		options, err := view.CompileRunViewOptions(opts...)
		if err != nil {
			return nil, err
		}
		compiled <- options
		return nil, nil
	}

	viewpkg.RunView(ctx, fake, nil, view.AsResponder(session))

	var options *view.RunViewOptions
	select {
	case options = <-compiled:
	case <-time.After(2 * time.Second):
		t.Fatal("RunView never invoked the view context")
	}

	if options.SameContext {
		t.Error("RunView must not set SameContext; the view has to run in a child view context")
	}
	if options.Session != session {
		t.Error("RunView must preserve the session set by view.AsResponder")
	}
	if options.Ctx != ctx {
		t.Error("RunView must pass the supplied go context through")
	}
}

// TestRunView_CallerOptionWins asserts the go context injected by RunView is
// prepended, so a caller that explicitly passes its own context option still wins.
func TestRunView_CallerOptionWins(t *testing.T) { //nolint:paralleltest // uses goleak.VerifyNone; must run serially
	defer goleak.VerifyNone(t)

	// These must be two DISTINCT contexts. t.Context() returns the same context on
	// every call for a given t, so deriving callerCtx is what makes the assertion
	// below able to fail at all.
	ctx := t.Context()
	callerCtx, cancelCaller := context.WithCancel(t.Context())
	defer cancelCaller()
	if ctx == callerCtx {
		t.Fatal("test is vacuous: ctx and callerCtx must differ")
	}

	compiled := make(chan *view.RunViewOptions, 1)

	fake := &mock.Context{}
	fake.RunViewStub = func(v view.View, opts ...view.RunViewOption) (any, error) {
		options, err := view.CompileRunViewOptions(opts...)
		if err != nil {
			return nil, err
		}
		compiled <- options
		return nil, nil
	}

	viewpkg.RunView(ctx, fake, nil, view.WithGoContext(callerCtx))

	select {
	case options := <-compiled:
		if options.Ctx != callerCtx {
			t.Error("an explicit caller context option must take precedence over RunView's")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunView never invoked the view context")
	}
}
