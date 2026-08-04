/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// ctxCapturingVault records the context handed to Statuses so the test can inspect
// whether it is linked to the runStatusListener loop context.
type ctxCapturingVault struct {
	called chan context.Context
}

func (v *ctxCapturingVault) Statuses(ctx context.Context, _ ...driver.TxID) ([]driver.TxValidationStatus[int], error) {
	select {
	case v.called <- ctx:
	default:
	}
	return nil, nil
}

func TestFinalityManager_StatusListenerContextPropagates(t *testing.T) {
	t.Parallel()

	vault := &ctxCapturingVault{called: make(chan context.Context, 1)}
	listenerManager := newFinalityListenerManager[int](logging.MustGetLogger(), &noop.Tracer{})
	manager := NewFinalityManager[int](listenerManager, logging.MustGetLogger(), vault, noop.NewTracerProvider(), 10)
	require.NoError(t, manager.AddListener("txID", &MockFinalityListener{}))

	loopCtx, cancelLoop := context.WithCancel(context.Background())
	go manager.runStatusListener(loopCtx)

	var captured context.Context
	select {
	case captured = <-vault.called:
	case <-time.After(5 * time.Second):
		t.Fatal("vault.Statuses was never called")
	}

	cancelLoop()
	require.Eventually(t, func() bool { return captured.Err() != nil }, 2*time.Second, 20*time.Millisecond,
		"context passed to Statuses must be derived from the loop ctx; see fsc-memory.md Issue #12")
}
