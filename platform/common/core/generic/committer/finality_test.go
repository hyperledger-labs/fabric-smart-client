/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/otel/trace/noop"
)

type MockVault struct {
	mock.Mock
}

func (m *MockVault) Statuses(ids ...string) ([]driver.TxValidationStatus[int], error) {
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
	listenerManager := newFinalityListenerManager[int](flogging.MustGetLogger("committer"), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, flogging.MustGetLogger("committer"), vault, noop.NewTracerProvider(), 10)
	listener := &MockFinalityListener{}

	err := manager.AddListener("txID", listener)
	assert.NoError(t, err)
	assert.Equal(t, manager.txIDs.Length(), 1)
	assert.True(t, manager.txIDs.Contains("txID"))

	// Adding listener with empty txID should return an error
	err = manager.AddListener("", listener)
	assert.Error(t, err)
	assert.Equal(t, manager.txIDs.Length(), 1)
}

func TestFinalityManager_RemoveListener(t *testing.T) {
	listenerManager := newFinalityListenerManager[int](flogging.MustGetLogger("committer"), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, flogging.MustGetLogger("committer"), vault, noop.NewTracerProvider(), 10)
	listener := &MockFinalityListener{}

	assert.NoError(t, manager.AddListener("txID", listener))

	manager.RemoveListener("txID", listener)
	assert.True(t, manager.txIDs.Empty())

	// Removing non-existing listener should do nothing
	manager.RemoveListener("non-existing", listener)
	assert.True(t, manager.txIDs.Empty())
}

func TestFinalityManager_Run(t *testing.T) {
	listenerManager := newFinalityListenerManager[int](flogging.MustGetLogger("committer"), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, flogging.MustGetLogger("committer"), vault, noop.NewTracerProvider(), 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.Run(ctx)

	time.Sleep(2 * time.Second) // Wait for some time to let the goroutines run
}

func TestFinalityManager_RunStatusListener(t *testing.T) {
	event := driver.FinalityEvent[int]{
		TxID:              "txID",
		ValidationCode:    1,
		ValidationMessage: "message",
	}

	vault := &MockVault{}
	listenerManager := newFinalityListenerManager[int](flogging.MustGetLogger("committer"), &noop.Tracer{})
	manager := NewFinalityManager[int](listenerManager, flogging.MustGetLogger("committer"), vault, noop.NewTracerProvider(), 10)
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
	assert.NoError(t, manager.AddListener("txID", listener))

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go manager.runEventQueue(ctx)
	manager.runStatusListener(ctx)
	listener.AssertExpectations(t)

	// Error case: Vault returns an error
	vault.On("Statuses", []string{"txID"}).Return(nil, errors.New("some error"))
	listener = &MockFinalityListener{}
	listener.On("OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage)
	assert.NoError(t, manager.AddListener("txID", listener))
	manager.listenerManager.(*finalityListenerManager[int]).txIDListeners["txID"] = []driver.FinalityListener[int]{listener}

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	manager.runStatusListener(ctx)
	listener.AssertNotCalled(t, "OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage)

	vault.AssertExpectations(t)
}

func TestFinalityManager_CloneListeners(t *testing.T) {
	listenerManager := newFinalityListenerManager[int](flogging.MustGetLogger("committer"), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, flogging.MustGetLogger("committer"), vault, noop.NewTracerProvider(), 10)
	listener := &MockFinalityListener{}
	assert.NoError(t, manager.AddListener("txID", listener))

	clone := listenerManager.cloneListeners("txID")
	assert.Len(t, clone, 1)
	assert.Equal(t, clone[0], listener)
}

func TestFinalityManager_Dispatch_PanicRecovery(t *testing.T) {
	listenerManager := newFinalityListenerManager[int](flogging.MustGetLogger("committer"), &noop.Tracer{})
	vault := &MockVault{}
	manager := NewFinalityManager[int](listenerManager, flogging.MustGetLogger("committer"), vault, noop.NewTracerProvider(), 10)
	listener := &MockFinalityListener{}
	event := driver.FinalityEvent[int]{
		Ctx:            context.TODO(),
		TxID:           "txID",
		ValidationCode: 1,
	}
	assert.NoError(t, manager.AddListener("txID", listener))

	listener.On("OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage).Once().Run(func(args mock.Arguments) {
		panic("listener panic")
	})
	assert.NotPanics(t, func() {
		manager.listenerManager.InvokeListeners(event)
	})
	listener.AssertExpectations(t)
}
