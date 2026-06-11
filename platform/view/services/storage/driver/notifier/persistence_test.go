/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package notifier

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/notifier/mock"
)

//go:generate counterfeiter -o mock/kvs.go --fake-name KeyValueStore github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver.KeyValueStore

// fakeIterator is a simple fake for testing
type fakeIterator struct{}

func (f *fakeIterator) Next() (*driver.UnversionedRead, error) {
	return nil, nil
}

func (f *fakeIterator) Close() {
}

var _ iterators.Iterator[*driver.UnversionedRead] = (*fakeIterator)(nil)

func TestNewUnversioned(t *testing.T) {
	t.Parallel()

	mockKVS := &mock.KeyValueStore{}
	upn := NewUnversioned(mockKVS)

	require.NotNil(t, upn)
	require.NotNil(t, upn.Persistence)
	require.NotNil(t, upn.Notifier)
	require.Equal(t, mockKVS, upn.Persistence)
}

func TestUnversionedPersistenceNotifier_SetState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ns             driver2.Namespace
		key            driver2.PKey
		val            driver2.RawValue
		setStateErr    error
		expectedOp     driver.Operation
		expectErr      bool
		expectEnqueued bool
	}{
		{
			name:           "successful update with value",
			ns:             "namespace1",
			key:            "key1",
			val:            []byte("value1"),
			setStateErr:    nil,
			expectedOp:     driver.Update,
			expectErr:      false,
			expectEnqueued: true,
		},
		{
			name:           "successful delete with empty value",
			ns:             "namespace1",
			key:            "key1",
			val:            []byte{},
			setStateErr:    nil,
			expectedOp:     driver.Delete,
			expectErr:      false,
			expectEnqueued: true,
		},
		{
			name:           "error from persistence",
			ns:             "namespace1",
			key:            "key1",
			val:            []byte("value1"),
			setStateErr:    errors.New("persistence error"),
			expectErr:      true,
			expectEnqueued: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockKVS := &mock.KeyValueStore{}
			mockKVS.SetStateReturns(tt.setStateErr)

			upn := NewUnversioned(mockKVS)

			err := upn.SetState(ctx, tt.ns, tt.key, tt.val)

			if tt.expectErr {
				require.Error(t, err)
				require.Empty(t, upn.Notifier.pending)
			} else {
				require.NoError(t, err)
				if tt.expectEnqueued {
					require.Len(t, upn.Notifier.pending, 1)
					require.Equal(t, tt.expectedOp, upn.Notifier.pending[0].operation)
					require.Equal(t, tt.ns, upn.Notifier.pending[0].payload["ns"])
					require.Equal(t, tt.key, upn.Notifier.pending[0].payload["pkey"])
				}
			}

			require.Equal(t, 1, mockKVS.SetStateCallCount())
		})
	}
}

func TestUnversionedPersistenceNotifier_SetStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ns             driver2.Namespace
		kvs            map[driver2.PKey]driver2.RawValue
		persistenceErr map[driver2.PKey]error
		expectedEvents int
	}{
		{
			name: "all successful updates",
			ns:   "namespace1",
			kvs: map[driver2.PKey]driver2.RawValue{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
			},
			persistenceErr: map[driver2.PKey]error{},
			expectedEvents: 2,
		},
		{
			name: "mixed success and failure",
			ns:   "namespace1",
			kvs: map[driver2.PKey]driver2.RawValue{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
				"key3": []byte("value3"),
			},
			persistenceErr: map[driver2.PKey]error{
				"key2": errors.New("error"),
			},
			expectedEvents: 2,
		},
		{
			name: "delete operations with empty values",
			ns:   "namespace1",
			kvs: map[driver2.PKey]driver2.RawValue{
				"key1": []byte{},
				"key2": []byte{},
			},
			persistenceErr: map[driver2.PKey]error{},
			expectedEvents: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockKVS := &mock.KeyValueStore{}
			mockKVS.SetStatesReturns(tt.persistenceErr)

			upn := NewUnversioned(mockKVS)

			errs := upn.SetStates(ctx, tt.ns, tt.kvs)

			require.Equal(t, tt.persistenceErr, errs)
			require.Len(t, upn.Notifier.pending, tt.expectedEvents)
			require.Equal(t, 1, mockKVS.SetStatesCallCount())
		})
	}
}

func TestUnversionedPersistenceNotifier_DeleteState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ns             driver2.Namespace
		key            driver2.PKey
		deleteErr      error
		expectErr      bool
		expectEnqueued bool
	}{
		{
			name:           "successful delete",
			ns:             "namespace1",
			key:            "key1",
			deleteErr:      nil,
			expectErr:      false,
			expectEnqueued: true,
		},
		{
			name:           "error from persistence",
			ns:             "namespace1",
			key:            "key1",
			deleteErr:      errors.New("delete error"),
			expectErr:      true,
			expectEnqueued: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockKVS := &mock.KeyValueStore{}
			mockKVS.DeleteStateReturns(tt.deleteErr)

			upn := NewUnversioned(mockKVS)

			err := upn.DeleteState(ctx, tt.ns, tt.key)

			if tt.expectErr {
				require.Error(t, err)
				require.Empty(t, upn.Notifier.pending)
			} else {
				require.NoError(t, err)
				if tt.expectEnqueued {
					require.Len(t, upn.Notifier.pending, 1)
					require.Equal(t, driver.Delete, upn.Notifier.pending[0].operation)
					require.Equal(t, tt.ns, upn.Notifier.pending[0].payload["ns"])
					require.Equal(t, tt.key, upn.Notifier.pending[0].payload["pkey"])
				}
			}

			require.Equal(t, 1, mockKVS.DeleteStateCallCount())
		})
	}
}

func TestUnversionedPersistenceNotifier_DeleteStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ns             driver2.Namespace
		keys           []driver2.PKey
		persistenceErr map[driver2.PKey]error
		expectedEvents int
	}{
		{
			name:           "all successful deletes",
			ns:             "namespace1",
			keys:           []driver2.PKey{"key1", "key2", "key3"},
			persistenceErr: map[driver2.PKey]error{},
			expectedEvents: 3,
		},
		{
			name: "mixed success and failure",
			ns:   "namespace1",
			keys: []driver2.PKey{"key1", "key2", "key3"},
			persistenceErr: map[driver2.PKey]error{
				"key2": errors.New("error"),
			},
			expectedEvents: 2,
		},
		{
			name: "all failures",
			ns:   "namespace1",
			keys: []driver2.PKey{"key1", "key2"},
			persistenceErr: map[driver2.PKey]error{
				"key1": errors.New("error1"),
				"key2": errors.New("error2"),
			},
			expectedEvents: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			mockKVS := &mock.KeyValueStore{}
			mockKVS.DeleteStatesReturns(tt.persistenceErr)

			upn := NewUnversioned(mockKVS)

			errs := upn.DeleteStates(ctx, tt.ns, tt.keys...)

			require.Equal(t, tt.persistenceErr, errs)
			require.Len(t, upn.Notifier.pending, tt.expectedEvents)
			require.Equal(t, 1, mockKVS.DeleteStatesCallCount())
		})
	}
}

func TestUnversionedPersistenceNotifier_Commit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		commitErr error
		expectErr bool
	}{
		{
			name:      "successful commit",
			commitErr: nil,
			expectErr: false,
		},
		{
			name:      "error from persistence",
			commitErr: errors.New("commit error"),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockKVS := &mock.KeyValueStore{}
			mockKVS.CommitReturns(tt.commitErr)

			upn := NewUnversioned(mockKVS)
			upn.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})

			err := upn.Commit()

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Empty(t, upn.Notifier.pending)
			}

			require.Equal(t, 1, mockKVS.CommitCallCount())
		})
	}
}

func TestUnversionedPersistenceNotifier_Discard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		discardErr error
		expectErr  bool
	}{
		{
			name:       "successful discard",
			discardErr: nil,
			expectErr:  false,
		},
		{
			name:       "error from persistence",
			discardErr: errors.New("discard error"),
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockKVS := &mock.KeyValueStore{}
			mockKVS.DiscardReturns(tt.discardErr)

			upn := NewUnversioned(mockKVS)
			upn.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})

			err := upn.Discard()

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Empty(t, upn.Notifier.pending)
			}

			require.Equal(t, 1, mockKVS.DiscardCallCount())
		})
	}
}

func TestUnversionedPersistenceNotifier_GetState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockKVS := &mock.KeyValueStore{}
	expectedValue := driver2.RawValue([]byte("value1"))
	mockKVS.GetStateReturns(expectedValue, nil)

	upn := NewUnversioned(mockKVS)

	value, err := upn.GetState(ctx, "namespace1", "key1")

	require.NoError(t, err)
	require.Equal(t, expectedValue, value)
	require.Equal(t, 1, mockKVS.GetStateCallCount())
}

func TestUnversionedPersistenceNotifier_GetStateRangeScanIterator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockKVS := &mock.KeyValueStore{}
	fakeIter := &fakeIterator{}
	mockKVS.GetStateRangeScanIteratorReturns(fakeIter, nil)

	upn := NewUnversioned(mockKVS)

	iter, err := upn.GetStateRangeScanIterator(ctx, "namespace1", "start", "end")

	require.NoError(t, err)
	require.Equal(t, fakeIter, iter)
	require.Equal(t, 1, mockKVS.GetStateRangeScanIteratorCallCount())
}

func TestUnversionedPersistenceNotifier_GetStateSetIterator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockKVS := &mock.KeyValueStore{}
	fakeIter := &fakeIterator{}
	mockKVS.GetStateSetIteratorReturns(fakeIter, nil)

	upn := NewUnversioned(mockKVS)

	iter, err := upn.GetStateSetIterator(ctx, "namespace1", "key1", "key2")

	require.NoError(t, err)
	require.Equal(t, fakeIter, iter)
	require.Equal(t, 1, mockKVS.GetStateSetIteratorCallCount())
}

func TestUnversionedPersistenceNotifier_Close(t *testing.T) {
	t.Parallel()

	mockKVS := &mock.KeyValueStore{}
	mockKVS.CloseReturns(nil)

	upn := NewUnversioned(mockKVS)

	err := upn.Close()

	require.NoError(t, err)
	require.Equal(t, 1, mockKVS.CloseCallCount())
}

func TestUnversionedPersistenceNotifier_BeginUpdate(t *testing.T) {
	t.Parallel()

	mockKVS := &mock.KeyValueStore{}
	mockKVS.BeginUpdateReturns(nil)

	upn := NewUnversioned(mockKVS)

	err := upn.BeginUpdate()

	require.NoError(t, err)
	require.Equal(t, 1, mockKVS.BeginUpdateCallCount())
}

func TestUnversionedPersistenceNotifier_Stats(t *testing.T) {
	t.Parallel()

	mockKVS := &mock.KeyValueStore{}
	expectedStats := map[string]interface{}{"count": 42}
	mockKVS.StatsReturns(expectedStats)

	upn := NewUnversioned(mockKVS)

	stats := upn.Stats()

	require.Equal(t, expectedStats, stats)
	require.Equal(t, 1, mockKVS.StatsCallCount())
}

func TestUnversionedPersistenceNotifier_Subscribe(t *testing.T) {
	t.Parallel()

	mockKVS := &mock.KeyValueStore{}
	upn := NewUnversioned(mockKVS)

	called := false
	callback := func(driver.Operation, map[driver.ColumnKey]string) {
		called = true
	}

	err := upn.Subscribe(callback)

	require.NoError(t, err)
	require.Len(t, upn.Notifier.listeners, 1)

	upn.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})
	upn.Notifier.Commit()

	require.True(t, called)
}

func TestUnversionedPersistenceNotifier_UnsubscribeAll(t *testing.T) {
	t.Parallel()

	mockKVS := &mock.KeyValueStore{}
	upn := NewUnversioned(mockKVS)

	err := upn.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {})
	require.NoError(t, err)
	require.Len(t, upn.Notifier.listeners, 1)

	err = upn.UnsubscribeAll()

	require.NoError(t, err)
	require.Empty(t, upn.Notifier.listeners)
}
