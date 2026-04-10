/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

type MockVault struct {
	mock.Mock
}

func (m *MockVault) Statuses(_ context.Context, ids ...driver.TxID) ([]driver.TxValidationStatus[int], error) {
	args := m.Called(ids)
	return args.Get(0).([]driver.TxValidationStatus[int]), args.Error(1)
}

type MockFinalityListener struct {
	mock.Mock
}

func (m *MockFinalityListener) OnStatus(_ context.Context, txID driver.TxID, status int, statusMessage string) {
	m.Called(txID, status, statusMessage)
}

func TestFinalityManager_AddListener(t *testing.T) {
	t.Parallel()
	listenerManager := newFinalityListenerManager[int](logging.MustGetLogger(), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, logging.MustGetLogger(), vault, noop.NewTracerProvider(), 10)
	listener := &MockFinalityListener{}

	err := manager.AddListener("txID", listener)
	require.NoError(t, err)
	require.Len(t, manager.listenerManager.TxIDs(), 1)
	require.Contains(t, manager.listenerManager.TxIDs(), "txID")

	// Adding listener with empty txID should return an error
	err = manager.AddListener("", listener)
	require.Error(t, err)
	require.Len(t, manager.listenerManager.TxIDs(), 1)
}

func TestFinalityManager_RemoveListener(t *testing.T) {
	t.Parallel()
	listenerManager := newFinalityListenerManager[int](logging.MustGetLogger(), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, logging.MustGetLogger(), vault, noop.NewTracerProvider(), 10)
	listener := &MockFinalityListener{}

	require.NoError(t, manager.AddListener("txID", listener))

	manager.RemoveListener("txID", listener)
	require.Empty(t, manager.listenerManager.TxIDs())

	// Removing non-existing listener should do nothing
	manager.RemoveListener("non-existing", listener)
	require.Empty(t, manager.listenerManager.TxIDs())
}

func TestFinalityManager_Run(t *testing.T) {
	t.Parallel()
	listenerManager := newFinalityListenerManager[int](logging.MustGetLogger(), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, logging.MustGetLogger(), vault, noop.NewTracerProvider(), 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.Run(ctx)

	time.Sleep(2 * time.Second) // Wait for some time to let the goroutines run
}

func TestFinalityManager_RunStatusListener(t *testing.T) {
	t.Parallel()
	event := driver.FinalityEvent[int]{
		TxID:              "txID",
		ValidationCode:    1,
		ValidationMessage: "message",
	}

	vault := &MockVault{}
	listenerManager := newFinalityListenerManager[int](logging.MustGetLogger(), &noop.Tracer{})
	manager := NewFinalityManager[int](listenerManager, logging.MustGetLogger(), vault, noop.NewTracerProvider(), 10)
	manager.postStatuses = collections.NewSet(1)

	// no listeners
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	manager.runStatusListener(ctx)

	// with listeners
	vault.On("Statuses", []string{"txID"}).Return([]driver.TxValidationStatus[int]{{
		TxID:           "txID",
		ValidationCode: 1,
		Message:        "message",
	}}, nil)
	listener := &MockFinalityListener{}
	listener.On("OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage).Once()
	require.NoError(t, manager.AddListener("txID", listener))

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go manager.runEventQueue(ctx)
	manager.runStatusListener(ctx)
	listener.AssertExpectations(t)

	// Error case: Vault returns an error
	vault.On("Statuses", []string{"txID"}).Return(nil, errors.New("some error"))
	listener = &MockFinalityListener{}
	listener.On("OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage)
	require.NoError(t, manager.AddListener("txID", listener))
	manager.listenerManager.(*finalityListenerManager[int]).txIDListeners["txID"] = []driver.FinalityListener[int]{listener}

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	manager.runStatusListener(ctx)
	listener.AssertNotCalled(t, "OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage)

	vault.AssertExpectations(t)
}

func TestFinalityManager_CloneListeners(t *testing.T) {
	t.Parallel()
	listenerManager := newFinalityListenerManager[int](logging.MustGetLogger(), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, logging.MustGetLogger(), vault, noop.NewTracerProvider(), 10)
	listener := &MockFinalityListener{}
	require.NoError(t, manager.AddListener("txID", listener))

	clone := listenerManager.cloneListeners("txID")
	require.Len(t, clone, 1)
	require.Equal(t, clone[0], listener)
}

func TestFinalityManager_Dispatch_PanicRecovery(t *testing.T) {
	t.Parallel()
	listenerManager := newFinalityListenerManager[int](logging.MustGetLogger(), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, logging.MustGetLogger(), vault, noop.NewTracerProvider(), 10)
	listener := &MockFinalityListener{}
	event := driver.FinalityEvent[int]{
		Ctx:            context.TODO(),
		TxID:           "txID",
		ValidationCode: 1,
	}
	require.NoError(t, manager.AddListener("txID", listener))

	listener.On("OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage).Once().Run(func(args mock.Arguments) {
		panic("listener panic")
	})
	require.NotPanics(t, func() {
		manager.listenerManager.InvokeListeners(event)
	})
	listener.AssertExpectations(t)
}
