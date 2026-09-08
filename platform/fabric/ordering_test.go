/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

func TestOrdering(t *testing.T) {
	t.Parallel()

	mockFNS := &mock.FabricNetworkService{}
	mockOrderer := &mock.Ordering{}
	mockFNS.OrderingServiceReturns(mockOrderer)

	ordering := &Ordering{network: mockFNS}

	ctx := t.Context()

	// Test Envelope case
	mockEnv := &mock.Envelope{}
	env := &Envelope{Envelope: mockEnv}

	mockOrderer.BroadcastReturns(nil)
	require.NoError(t, ordering.Broadcast(ctx, env))
	require.Equal(t, 1, mockOrderer.BroadcastCallCount())
	c, b := mockOrderer.BroadcastArgsForCall(0)
	require.Equal(t, ctx, c)
	require.Equal(t, mockEnv, b)

	// Test Transaction case
	mockTx := &mock.Transaction{}
	tx := &Transaction{tx: mockTx}
	require.NoError(t, ordering.Broadcast(ctx, tx))
	require.Equal(t, 2, mockOrderer.BroadcastCallCount())
	c, b = mockOrderer.BroadcastArgsForCall(1)
	require.Equal(t, ctx, c)
	require.Equal(t, mockTx, b)

	// Test default case
	require.NoError(t, ordering.Broadcast(ctx, "something else"))
	require.Equal(t, 3, mockOrderer.BroadcastCallCount())
	c, b = mockOrderer.BroadcastArgsForCall(2)
	require.Equal(t, ctx, c)
	require.Equal(t, "something else", b)

	// Test error
	mockOrderer.BroadcastReturns(errors.New("broadcast err"))
	err := ordering.Broadcast(ctx, "fail")
	require.ErrorContains(t, err, "broadcast err")
}
