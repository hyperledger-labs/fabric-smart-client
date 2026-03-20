/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"context"
	"reflect"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger/mock"
	"github.com/stretchr/testify/require"
)

func TestProvider_Initialize(t *testing.T) {
	p := NewProvider(nil, nil)
	ctx := context.Background()
	p.Initialize(ctx)
	require.Equal(t, ctx, p.baseCtx)

	// Second initialization should not change anything
	ctx2, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.Initialize(ctx2)
	require.Equal(t, ctx, p.baseCtx)
}

func TestProvider_NewLedger(t *testing.T) {
	p := NewProvider(nil, nil)

	// Should panic if not initialized
	require.Panics(t, func() {
		_, _ = p.NewLedger("test-net", "test-ch")
	})

	ctx := context.Background()
	p.Initialize(ctx)

	fakeLedger := &mockLedger{}
	p.newLedger = func(network, channel string, provider *Provider) (driver.Ledger, error) {
		return fakeLedger, nil
	}

	l, err := p.NewLedger("test-net", "test-ch")
	require.NoError(t, err)
	require.Equal(t, fakeLedger, l)

	// Test singleton behavior
	l2, err := p.NewLedger("test-net", "test-ch")
	require.NoError(t, err)
	require.Equal(t, l, l2)
}

func TestGetLedgerProvider(t *testing.T) {
	fakeSP := &mock.FakeServicesProvider{}
	p := NewProvider(nil, nil)

	fakeSP.GetServiceReturns(p, nil)

	lp, err := GetLedgerProvider(fakeSP)
	require.NoError(t, err)
	require.Equal(t, p, lp)

	require.Equal(t, 1, fakeSP.GetServiceCallCount())
	arg := fakeSP.GetServiceArgsForCall(0)
	require.Equal(t, reflect.TypeOf((*Provider)(nil)), arg)
}

type mockLedger struct {
	driver.Ledger
}
