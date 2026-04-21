/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"errors"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mocks"
	"github.com/stretchr/testify/require"
)

func TestNewFactory(t *testing.T) {
	t.Parallel()

	fns := &mocks.FakeFabricNetworkService{}
	factory := NewFactory(fns)

	require.NotNil(t, factory)
	require.Same(t, fns, factory.fns)
}

func TestFactoryNewTransaction(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	creator := []byte("creator")

	tests := []struct {
		name                 string
		channelName          string
		nonce                []byte
		txID                 driver.TxID
		networkName          string
		channelErr           error
		expectedErr          string
		expectGeneratedNonce bool
		expectGeneratedTxID  bool
		expectedChannelCalls int
		expectedNameCalls    int
	}{
		{
			name:                 "channel lookup fails",
			channelName:          "test-channel",
			channelErr:           errors.New("channel lookup failed"),
			expectedErr:          "channel lookup failed",
			expectedChannelCalls: 1,
			expectedNameCalls:    0,
		},
		{
			name:                 "generates nonce and txid when missing",
			channelName:          "test-channel",
			networkName:          "test-network",
			expectGeneratedNonce: true,
			expectGeneratedTxID:  true,
			expectedChannelCalls: 1,
			expectedNameCalls:    1,
		},
		{
			name:                 "uses provided nonce and txid",
			channelName:          "explicit-channel",
			nonce:                []byte("explicit-nonce"),
			txID:                 driver.TxID("tx-id-123"),
			networkName:          "explicit-network",
			expectedChannelCalls: 1,
			expectedNameCalls:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fns := &mocks.FakeFabricNetworkService{}
			fns.NameReturns(tc.networkName)
			fns.ChannelReturns((*mocks.FakeChannel)(nil), tc.channelErr)
			factory := NewFactory(fns)

			rawTx, err := factory.NewTransaction(ctx, tc.channelName, tc.nonce, creator, tc.txID, []byte("ignored request"))

			if tc.expectedErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErr)
				require.Nil(t, rawTx)
				require.Equal(t, tc.expectedChannelCalls, fns.ChannelCallCount())
				require.Equal(t, tc.channelName, fns.ChannelArgsForCall(0))
				require.Equal(t, tc.expectedNameCalls, fns.NameCallCount())
				return
			}

			require.NoError(t, err)
			tx, ok := rawTx.(*Transaction)
			require.True(t, ok)
			require.Equal(t, ctx, tx.ctx)
			require.Same(t, fns, tx.fns)
			require.Equal(t, creator, []byte(tx.TCreator))
			require.Equal(t, tc.networkName, tx.TNetwork)
			require.Equal(t, tc.channelName, tx.TChannel)
			require.NotNil(t, tx.TTransient)
			require.Empty(t, tx.TTransient)
			// TODO: the following 2 values should be fixed with the TODOs changed from factory.go
			require.Equal(t, "1", tx.TChaincodeVersion)
			require.Equal(t, "iou", tx.TChaincode)
			require.Equal(t, tc.expectedChannelCalls, fns.ChannelCallCount())
			require.Equal(t, tc.channelName, fns.ChannelArgsForCall(0))
			require.Equal(t, tc.expectedNameCalls, fns.NameCallCount())

			if tc.expectGeneratedNonce {
				require.NotEmpty(t, tx.TNonce)
			} else {
				require.Equal(t, tc.nonce, tx.TNonce)
			}

			if tc.expectGeneratedTxID {
				require.NotEmpty(t, tx.TTxID)
			} else {
				require.Equal(t, tc.txID, tx.TTxID)
			}
		})
	}
}

func TestFactoryNewTransactionCachedIdentitiesConfig(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	creator := []byte("creator")

	t.Run("defaults to true when ConfigService is nil", func(t *testing.T) {
		t.Parallel()
		fns := &mocks.FakeFabricNetworkService{}
		fns.NameReturns("test-network")
		fns.ChannelReturns((*mocks.FakeChannel)(nil), nil)
		// ConfigService returns nil (default zero value)
		factory := NewFactory(fns)

		rawTx, err := factory.NewTransaction(ctx, "ch1", []byte("nonce"), creator, "tx1", nil)
		require.NoError(t, err)
		tx := rawTx.(*Transaction)
		require.True(t, tx.useCachedIdentities)
	})

	t.Run("defaults to true when key is not set", func(t *testing.T) {
		t.Parallel()
		fns := &mocks.FakeFabricNetworkService{}
		fns.NameReturns("test-network")
		fns.ChannelReturns((*mocks.FakeChannel)(nil), nil)
		fns.ConfigServiceReturns(&stubConfigService{values: map[string]interface{}{}})
		factory := NewFactory(fns)

		rawTx, err := factory.NewTransaction(ctx, "ch1", []byte("nonce"), creator, "tx1", nil)
		require.NoError(t, err)
		tx := rawTx.(*Transaction)
		require.True(t, tx.useCachedIdentities)
	})

	t.Run("respects false override from config", func(t *testing.T) {
		t.Parallel()
		fns := &mocks.FakeFabricNetworkService{}
		fns.NameReturns("test-network")
		fns.ChannelReturns((*mocks.FakeChannel)(nil), nil)
		fns.ConfigServiceReturns(&stubConfigService{
			values: map[string]interface{}{"fabric-x.endorser.useCachedIdentities": false},
		})
		factory := NewFactory(fns)

		rawTx, err := factory.NewTransaction(ctx, "ch1", []byte("nonce"), creator, "tx1", nil)
		require.NoError(t, err)
		tx := rawTx.(*Transaction)
		require.False(t, tx.useCachedIdentities)
	})

	t.Run("supports legacy key for backward compatibility", func(t *testing.T) {
		t.Parallel()
		fns := &mocks.FakeFabricNetworkService{}
		fns.NameReturns("test-network")
		fns.ChannelReturns((*mocks.FakeChannel)(nil), nil)
		fns.ConfigServiceReturns(&stubConfigService{
			values: map[string]interface{}{"endorser.useCachedIdentities": false},
		})
		factory := NewFactory(fns)

		rawTx, err := factory.NewTransaction(ctx, "ch1", []byte("nonce"), creator, "tx1", nil)
		require.NoError(t, err)
		tx := rawTx.(*Transaction)
		require.False(t, tx.useCachedIdentities)
	})
}

// stubConfigService is a minimal ConfigService implementation for config tests.
type stubConfigService struct {
	fdriver.ConfigService
	values map[string]interface{}
}

func (s *stubConfigService) IsSet(key string) bool {
	_, ok := s.values[key]
	return ok
}

func (s *stubConfigService) GetBool(key string) bool {
	v, ok := s.values[key]
	if !ok {
		return false
	}
	return v.(bool)
}
