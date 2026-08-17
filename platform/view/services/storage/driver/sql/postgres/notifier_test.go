/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// newTestNotifier builds a Notifier for exercising the subscriber bookkeeping
// (Subscribe/UnsubscribeAll/Close/dispatch) without a database behind it.
//
// It wires up the lifecycle context the same way NewNotifier does, and consumes
// startOnce up front so Subscribe never tries to start a listen loop — there is no
// listener here. This keeps the nil-listener/nil-cancel concern in the tests, where
// it belongs, rather than as guards on the production paths.
func newTestNotifier(t *testing.T) *Notifier {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	n := &Notifier{table: "test-table", ctx: ctx, cancel: cancel}
	n.startOnce.Do(func() {})
	return n
}

func TestNotifier_UnsubscribeAll_DoesNotPanicOnDispatch(t *testing.T) {
	t.Parallel()
	n := newTestNotifier(t)
	called := 0
	require.NoError(t, n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) { called++ }))
	require.NoError(t, n.UnsubscribeAll())
	require.NotPanics(t, func() { n.dispatch(driver.Insert, nil) })
	require.Equal(t, 0, called)
}

func TestNotifier_CloseThenDispatch_DoesNotPanic(t *testing.T) {
	t.Parallel()
	n := newTestNotifier(t)
	called := 0
	require.NoError(t, n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) { called++ }))
	require.NoError(t, n.Close())
	require.NotPanics(t, func() { n.dispatch(driver.Insert, nil) })
	require.Equal(t, 0, called)
}

func TestNotifier_SubscribeAfterClose_Errors(t *testing.T) {
	t.Parallel()
	n := newTestNotifier(t)
	require.NoError(t, n.Close())
	err := n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {})
	require.Error(t, err)
}

func TestNotifier_DispatchNotifiesSubscribers(t *testing.T) {
	t.Parallel()
	n := newTestNotifier(t)
	var got []driver.Operation
	require.NoError(t, n.Subscribe(func(op driver.Operation, _ map[driver.ColumnKey]string) {
		got = append(got, op)
	}))
	n.dispatch(driver.Insert, nil)
	n.dispatch(driver.Delete, nil)
	require.Equal(t, []driver.Operation{driver.Insert, driver.Delete}, got)
}
