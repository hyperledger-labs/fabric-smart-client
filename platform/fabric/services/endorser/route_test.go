/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
)

// envelopeRaw returns a marshaled TransactionEnvelope of a Fabric transaction on the channel
// and genesis hash of key.
func envelopeRaw(t *testing.T, key *driver.ChannelKey) []byte {
	t.Helper()
	payload, err := proto.Marshal(&protos.Transaction{TxId: "txid"})
	require.NoError(t, err)
	raw, err := proto.Marshal(&protos.TransactionEnvelope{
		Platform:           key.Platform,
		Channel:            key.Channel,
		ChannelGenesisHash: key.GenesisHash,
		Payload:            payload,
	})
	require.NoError(t, err)
	return raw
}

func TestRouteReceivedEnvelope(t *testing.T) {
	t.Parallel()
	for name, key := range map[string]*driver.ChannelKey{"default network": net1Key, "other network": net2Key} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReceiveFixture(t, []byte("bob"))
			plainNet, plainKey, err := routeReceived(f.ctx, "", keyedRaw(t, key))
			require.NoError(t, err)
			envNet, envKey, err := routeReceived(f.ctx, "", envelopeRaw(t, key))
			require.NoError(t, err)
			require.Equal(t, plainNet, envNet)
			require.Equal(t, plainKey, envKey)
		})
	}

	t.Run("unknown key", func(t *testing.T) {
		t.Parallel()
		f := newReceiveFixture(t, []byte("bob"))
		_, _, err := routeReceived(f.ctx, "", envelopeRaw(t, &driver.ChannelKey{Platform: "generic", Channel: "ch1", GenesisHash: []byte("other")}))
		require.ErrorContains(t, err, "no local network matches channel [ch1]")
	})
}

// The builder hands the transaction manager the bytes it received, so the manager checks that
// an envelope is of its own platform; routing reads only the key. A keyless envelope goes to
// the default network, whose manager refuses one of another platform (see the managers'
// TestNewTransactionFromBytesRejectsOtherPlatform tests).
func TestNewTransactionFromBytes_ManagerReceivesEnvelope(t *testing.T) {
	t.Parallel()
	f := newReceiveFixture(t, []byte("bob"))
	env := envelopeRaw(t, &driver.ChannelKey{Platform: "generic", Channel: "ch1"})

	_, _, err := NewTransactionFromBytes(f.ctx, env)
	require.NoError(t, err)
	require.Equal(t, 1, f.tm.NewTransactionFromBytesCallCount())
	_, _, got := f.tm.NewTransactionFromBytesArgsForCall(0)
	require.Equal(t, env, got)
}
