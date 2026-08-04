/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

func TestNotifier_UnsubscribeAll_DoesNotPanicOnDispatch(t *testing.T) {
	t.Parallel()
	n := &Notifier{}
	called := 0
	require.NoError(t, n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) { called++ }))
	require.NoError(t, n.UnsubscribeAll())
	require.NotPanics(t, func() { n.dispatch(driver.Insert, nil) })
	require.Equal(t, 0, called)
}

func TestNotifier_Unsubscribe_RemovesOne(t *testing.T) {
	t.Parallel()
	n := &Notifier{}
	a, b := 0, 0
	callbackA := func(driver.Operation, map[driver.ColumnKey]string) { a++ }
	callbackB := func(driver.Operation, map[driver.ColumnKey]string) { b++ }
	require.NoError(t, n.Subscribe(callbackA))
	require.NoError(t, n.Subscribe(callbackB))
	require.NoError(t, n.Unsubscribe(callbackB))
	n.dispatch(driver.Insert, nil)
	require.Equal(t, 1, a)
	require.Equal(t, 0, b)
}

func TestNotifier_SubscribeAfterClose_Errors(t *testing.T) {
	t.Parallel()
	n := &Notifier{}
	require.NoError(t, n.Close())
	err := n.Subscribe(func(driver.Operation, map[driver.ColumnKey]string) {})
	require.Error(t, err)
}
