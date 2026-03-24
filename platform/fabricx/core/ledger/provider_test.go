/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestProvider_Initialize(t *testing.T) {
	mockGRPC := &mock.GRPCClientProvider{}
	mockQS := &mock.QueryServiceProvider{}
	p := ledger.NewProvider(mockGRPC, mockQS)
	ctx := context.Background()
	p.Initialize(ctx)
	require.Equal(t, ctx, p.Context())

	// Second initialization should not change anything
	ctx2, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Initialize(ctx2)
	require.Equal(t, ctx, p.Context())
}

func TestProvider_NewLedger(t *testing.T) {
	mockGRPC := &mock.GRPCClientProvider{}
	mockQS := &mock.QueryServiceProvider{}
	mockGRPC.NotificationServiceClientReturns(&grpc.ClientConn{}, nil)
	p := ledger.NewProvider(mockGRPC, mockQS)

	// Should panic if not initialized
	require.Panics(t, func() {
		_, _ = p.NewLedger("test-net", "test-ch")
	})

	ctx := context.Background()
	p.Initialize(ctx)

	// Test successful ledger creation
	l, err := p.NewLedger("test-net", "")
	require.NoError(t, err)
	require.NotNil(t, l)

	// Test singleton behavior: same network should reuse the same ledger
	// by checking that GRPCClientProvider.NotificationServiceClient is called only once
	l2, err := p.NewLedger("test-net", "")
	require.NoError(t, err)
	require.NotNil(t, l2)
	require.Equal(t, 1, mockGRPC.NotificationServiceClientCallCount())

	// Test different network results in another call
	l3, err := p.NewLedger("test-net2", "")
	require.NoError(t, err)
	require.NotNil(t, l3)
	require.Equal(t, 2, mockGRPC.NotificationServiceClientCallCount())
}

func TestGetLedgerProvider(t *testing.T) {
	fakeSP := &mock.ServicesProvider{}
	p := ledger.NewProvider(nil, nil)

	fakeSP.GetServiceReturns(p, nil)

	lp, err := ledger.GetLedgerProvider(fakeSP)
	require.NoError(t, err)
	require.Equal(t, p, lp)

	require.Equal(t, 1, fakeSP.GetServiceCallCount())
	arg := fakeSP.GetServiceArgsForCall(0)
	require.Equal(t, reflect.TypeOf((*ledger.Provider)(nil)), arg)
}

func TestProvider_NewLedger_GRPCClientError(t *testing.T) {
	mockGRPC := &mock.GRPCClientProvider{}
	mockQS := &mock.QueryServiceProvider{}
	expectedErr := errors.New("grpc connection error")
	mockGRPC.NotificationServiceClientReturns(nil, expectedErr)

	p := ledger.NewProvider(mockGRPC, mockQS)
	ctx := context.Background()
	p.Initialize(ctx)

	l, err := p.NewLedger("test-net", "")
	require.Error(t, err)
	require.Nil(t, l)
	require.Equal(t, expectedErr, err)
}

func TestGetLedgerProvider_Error(t *testing.T) {
	fakeSP := &mock.ServicesProvider{}
	expectedErr := errors.New("service not found")
	fakeSP.GetServiceReturns(nil, expectedErr)

	lp, err := ledger.GetLedgerProvider(fakeSP)
	require.Error(t, err)
	require.Nil(t, lp)
	require.ErrorContains(t, err, "could not find ledger provider")
}
