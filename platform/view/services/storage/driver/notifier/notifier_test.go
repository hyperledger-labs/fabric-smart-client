/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package notifier

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func TestNewNotifier(t *testing.T) {
	t.Parallel()

	n := NewNotifier()

	require.NotNil(t, n)
	require.NotNil(t, n.listeners)
	require.Empty(t, n.listeners)
	require.Empty(t, n.pending)
}

func TestNotifier_Subscribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		callbacks     []driver.TriggerCallback
		expectedCount int
	}{
		{
			name:          "single callback",
			callbacks:     []driver.TriggerCallback{func(driver.Operation, map[driver.ColumnKey]string) {}},
			expectedCount: 1,
		},
		{
			name: "multiple callbacks",
			callbacks: []driver.TriggerCallback{
				func(driver.Operation, map[driver.ColumnKey]string) {},
				func(driver.Operation, map[driver.ColumnKey]string) {},
				func(driver.Operation, map[driver.ColumnKey]string) {},
			},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := NewNotifier()
			for _, cb := range tt.callbacks {
				err := n.Subscribe(cb)
				require.NoError(t, err)
			}

			require.Len(t, n.listeners, tt.expectedCount)
		})
	}
}

func TestNotifier_UnsubscribeAll(t *testing.T) {
	t.Parallel()

	n := NewNotifier()

	// Add some callbacks
	err := n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {})
	require.NoError(t, err)
	err = n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {})
	require.NoError(t, err)
	require.Len(t, n.listeners, 2)

	// Unsubscribe all
	err = n.UnsubscribeAll()
	require.NoError(t, err)
	require.Empty(t, n.listeners)
}

func TestNotifier_EnqueueEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events []struct {
			operation driver.Operation
			payload   map[driver.ColumnKey]string
		}
		expectedCount int
	}{
		{
			name: "single insert event",
			events: []struct {
				operation driver.Operation
				payload   map[driver.ColumnKey]string
			}{
				{
					operation: driver.Insert,
					payload:   map[driver.ColumnKey]string{"pkey": "key1", "value": "val1"},
				},
			},
			expectedCount: 1,
		},
		{
			name: "multiple events",
			events: []struct {
				operation driver.Operation
				payload   map[driver.ColumnKey]string
			}{
				{
					operation: driver.Insert,
					payload:   map[driver.ColumnKey]string{"pkey": "key1"},
				},
				{
					operation: driver.Update,
					payload:   map[driver.ColumnKey]string{"pkey": "key2"},
				},
				{
					operation: driver.Delete,
					payload:   map[driver.ColumnKey]string{"pkey": "key3"},
				},
			},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := NewNotifier()
			for _, event := range tt.events {
				n.EnqueueEvent(event.operation, event.payload)
			}

			require.Len(t, n.pending, tt.expectedCount)
		})
	}
}

func TestNotifier_Commit(t *testing.T) {
	t.Parallel()

	t.Run("commit with no listeners", func(t *testing.T) {
		t.Parallel()

		n := NewNotifier()
		n.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})
		require.Len(t, n.pending, 1)

		n.Commit()
		require.Empty(t, n.pending)
	})

	t.Run("commit with single listener", func(t *testing.T) {
		t.Parallel()

		n := NewNotifier()
		var called bool
		var receivedOp driver.Operation
		var receivedPayload map[driver.ColumnKey]string

		err := n.Subscribe(func(op driver.Operation, payload map[driver.ColumnKey]string) {
			called = true
			receivedOp = op
			receivedPayload = payload
		})
		require.NoError(t, err)

		payload := map[driver.ColumnKey]string{"pkey": "key1", "value": "val1"}
		n.EnqueueEvent(driver.Insert, payload)
		n.Commit()

		require.True(t, called)
		require.Equal(t, driver.Insert, receivedOp)
		require.Equal(t, payload, receivedPayload)
		require.Empty(t, n.pending)
	})

	t.Run("commit with multiple listeners", func(t *testing.T) {
		t.Parallel()

		n := NewNotifier()
		callCount := 0
		var mu sync.Mutex

		callback := func(driver.Operation, map[driver.ColumnKey]string) {
			mu.Lock()
			callCount++
			mu.Unlock()
		}

		err := n.Subscribe(callback)
		require.NoError(t, err)
		err = n.Subscribe(callback)
		require.NoError(t, err)

		n.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})
		n.Commit()

		require.Equal(t, 2, callCount)
		require.Empty(t, n.pending)
	})

	t.Run("commit with multiple events", func(t *testing.T) {
		t.Parallel()

		n := NewNotifier()
		callCount := 0

		err := n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {
			callCount++
		})
		require.NoError(t, err)

		n.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})
		n.EnqueueEvent(driver.Update, map[driver.ColumnKey]string{"pkey": "key2"})
		n.EnqueueEvent(driver.Delete, map[driver.ColumnKey]string{"pkey": "key3"})
		n.Commit()

		require.Equal(t, 3, callCount)
		require.Empty(t, n.pending)
	})
}

func TestNotifier_Discard(t *testing.T) {
	t.Parallel()

	t.Run("discard pending events", func(t *testing.T) {
		t.Parallel()

		n := NewNotifier()
		n.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})
		n.EnqueueEvent(driver.Update, map[driver.ColumnKey]string{"pkey": "key2"})
		require.Len(t, n.pending, 2)

		n.Discard()
		require.Empty(t, n.pending)
	})

	t.Run("discard does not trigger listeners", func(t *testing.T) {
		t.Parallel()

		n := NewNotifier()
		called := false

		err := n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {
			called = true
		})
		require.NoError(t, err)

		n.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})
		n.Discard()

		require.False(t, called)
		require.Empty(t, n.pending)
	})
}

func TestNotifier_Operations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation driver.Operation
	}{
		{name: "insert operation", operation: driver.Insert},
		{name: "update operation", operation: driver.Update},
		{name: "delete operation", operation: driver.Delete},
		{name: "unknown operation", operation: driver.Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n := NewNotifier()
			var receivedOp driver.Operation

			err := n.Subscribe(func(op driver.Operation, _ map[driver.ColumnKey]string) {
				receivedOp = op
			})
			require.NoError(t, err)

			n.EnqueueEvent(tt.operation, map[driver.ColumnKey]string{"pkey": "key1"})
			n.Commit()

			require.Equal(t, tt.operation, receivedOp)
		})
	}
}

func TestNotifier_ConcurrentEnqueue(t *testing.T) {
	t.Parallel()

	n := NewNotifier()
	const goroutines = 10
	const eventsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for range eventsPerGoroutine {
				n.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{
					"pkey": "key",
					"id":   string(rune(id)),
				})
			}
		}(i)
	}

	wg.Wait()
	require.Len(t, n.pending, goroutines*eventsPerGoroutine)
}

func TestNotifier_ConcurrentSubscribe(t *testing.T) {
	t.Parallel()

	n := NewNotifier()
	const goroutines = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			err := n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {})
			require.NoError(t, err)
		}()
	}

	wg.Wait()
	require.Len(t, n.listeners, goroutines)
}

func TestNotifier_CommitAfterDiscard(t *testing.T) {
	t.Parallel()

	n := NewNotifier()
	called := false

	err := n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {
		called = true
	})
	require.NoError(t, err)

	n.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})
	n.Discard()
	n.Commit()

	require.False(t, called)
	require.Empty(t, n.pending)
}

func TestNotifier_MultipleCommits(t *testing.T) {
	t.Parallel()

	n := NewNotifier()
	callCount := 0

	err := n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {
		callCount++
	})
	require.NoError(t, err)

	// First commit
	n.EnqueueEvent(driver.Insert, map[driver.ColumnKey]string{"pkey": "key1"})
	n.Commit()
	require.Equal(t, 1, callCount)

	// Second commit
	n.EnqueueEvent(driver.Update, map[driver.ColumnKey]string{"pkey": "key2"})
	n.Commit()
	require.Equal(t, 2, callCount)

	// Third commit with no events
	n.Commit()
	require.Equal(t, 2, callCount)
}
