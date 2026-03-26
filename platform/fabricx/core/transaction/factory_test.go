/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
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

	ctx := context.Background()
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
		tc := tc
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
