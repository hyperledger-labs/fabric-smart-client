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

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	fake := &mock.Context{}
	fake.RunViewStub = func(v view.View, opts ...view.RunViewOption) (any, error) {
		options, err := view.CompileRunViewOptions(opts...)
		if err != nil {
			t.Errorf("failed compiling options: %v", err)
			return nil, err
		}
		if options.Ctx == nil {
			t.Error("expected view.WithContext(ctx) to be present in opts")
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
