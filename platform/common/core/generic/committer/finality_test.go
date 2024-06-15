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

func (m *MockFinalityListener) OnStatus(txID driver.TxID, status int, statusMessage string) {
	m.Called(txID, status, statusMessage)
}

func TestFinalityManager_AddListener(t *testing.T) {
	vault := &MockVault{}
	manager := NewFinalityManager[int](flogging.MustGetLogger("committer"), vault)
	listener := &MockFinalityListener{}

	err := manager.AddListener("txID", listener)
	assert.NoError(t, err)
	assert.Len(t, manager.txIDListeners, 1)
	assert.Contains(t, manager.txIDListeners, "txID")
	assert.Contains(t, manager.txIDListeners["txID"], listener)

	// Adding listener with empty txID should return an error
	err = manager.AddListener("", listener)
	assert.Error(t, err)
	assert.Len(t, manager.txIDListeners, 1)
}

func TestFinalityManager_RemoveListener(t *testing.T) {
	vault := &MockVault{}
	manager := NewFinalityManager[int](flogging.MustGetLogger("committer"), vault)
	listener := &MockFinalityListener{}

	assert.NoError(t, manager.AddListener("txID", listener))

	manager.RemoveListener("txID", listener)
	assert.Len(t, manager.txIDListeners, 0)

	// Removing non-existing listener should do nothing
	manager.RemoveListener("non-existing", listener)
	assert.Len(t, manager.txIDListeners, 0)
}

func TestFinalityManager_Run(t *testing.T) {
	vault := &MockVault{}
	manager := NewFinalityManager[int](flogging.MustGetLogger("committer"), vault)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager.Run(ctx)

	time.Sleep(2 * time.Second) // Wait for some time to let the goroutines run
}

func TestFinalityManager_RunStatusListener(t *testing.T) {
	event := FinalityEvent[int]{
		TxID:              "txID",
		ValidationCode:    1,
		ValidationMessage: "message",
	}

	vault := &MockVault{}
	manager := NewFinalityManager[int](flogging.MustGetLogger("committer"), vault)
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
	manager.txIDListeners["txID"] = []driver.FinalityListener[int]{listener}

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	manager.runStatusListener(ctx)
	listener.AssertNotCalled(t, "OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage)

	vault.AssertExpectations(t)
}

func TestFinalityManager_CloneListeners(t *testing.T) {
	vault := &MockVault{}
	manager := NewFinalityManager[int](flogging.MustGetLogger("committer"), vault)
	listener := &MockFinalityListener{}
	assert.NoError(t, manager.AddListener("txID", listener))

	clone := manager.cloneListeners("txID")
	assert.Len(t, clone, 1)
	assert.Equal(t, clone[0], listener)
}

func TestFinalityManager_TxIDs(t *testing.T) {
	vault := &MockVault{}
	manager := NewFinalityManager[int](flogging.MustGetLogger("committer"), vault)

	manager.txIDListeners["txID"] = []driver.FinalityListener[int]{}

	txIDs := manager.txIDs()
	assert.Len(t, txIDs, 1)
	assert.Equal(t, txIDs[0], "txID")
}

func TestFinalityManager_Dispatch_PanicRecovery(t *testing.T) {
	vault := &MockVault{}
	manager := NewFinalityManager[int](flogging.MustGetLogger("committer"), vault)
	listener := &MockFinalityListener{}
	event := FinalityEvent[int]{
		TxID:           "txID",
		ValidationCode: 1,
	}
	assert.NoError(t, manager.AddListener("txID", listener))

	listener.On("OnStatus", event.TxID, event.ValidationCode, event.ValidationMessage).Once().Run(func(args mock.Arguments) {
		panic("listener panic")
	})
	assert.NotPanics(t, func() {
		manager.Dispatch(event)
	})
	listener.AssertExpectations(t)
}
