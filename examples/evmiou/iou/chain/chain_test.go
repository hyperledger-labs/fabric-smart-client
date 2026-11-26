/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chain

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client/mock"
)

// testKey returns a reproducible secp256k1 test key.
func testKey(low byte) *secp256k1.PrivateKey {
	raw := make([]byte, 32)
	raw[31] = low

	return secp256k1.PrivKeyFromBytes(raw)
}

// testHash returns a reproducible, distinguishable transaction hash.
func testHash(low byte) client.Hash {
	var h client.Hash
	h[client.HashLength-1] = low

	return h
}

func TestWaitReceiptPendingUntilFound(t *testing.T) {
	t.Parallel()

	fake := &mock.EVMClient{}
	h := testHash(1)
	fake.GetTransactionReceiptReturnsOnCall(0, nil, nil)
	fake.GetTransactionReceiptReturnsOnCall(1, nil, nil)
	fake.GetTransactionReceiptReturnsOnCall(2, &client.Receipt{TxHash: h, Status: 1}, nil)

	receipt, err := WaitReceipt(context.Background(), fake, h, time.Millisecond, time.Second)

	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, uint64(1), receipt.Status)
	assert.Equal(t, 3, fake.GetTransactionReceiptCallCount())
}

func TestWaitReceiptReverted(t *testing.T) {
	t.Parallel()

	fake := &mock.EVMClient{}
	h := testHash(2)
	fake.GetTransactionReceiptReturns(&client.Receipt{TxHash: h, Status: 0}, nil)

	_, err := WaitReceipt(context.Background(), fake, h, time.Millisecond, time.Second)

	require.ErrorIs(t, err, ErrReverted)
	assert.Contains(t, err.Error(), h.Hex())
}

// TestWaitReceiptZeroPollDefaults checks that a non-positive poll interval does not panic
// (time.NewTicker panics on it) but falls back to defaultPollInterval instead.
func TestWaitReceiptZeroPollDefaults(t *testing.T) {
	t.Parallel()

	fake := &mock.EVMClient{}
	h := testHash(4)
	fake.GetTransactionReceiptReturns(&client.Receipt{TxHash: h, Status: 1}, nil)

	receipt, err := WaitReceipt(context.Background(), fake, h, 0, time.Second)

	require.NoError(t, err)
	require.NotNil(t, receipt)
}

func TestWaitReceiptTimeout(t *testing.T) {
	t.Parallel()

	fake := &mock.EVMClient{}
	h := testHash(3)
	fake.GetTransactionReceiptReturns(nil, nil)

	_, err := WaitReceipt(context.Background(), fake, h, 10*time.Millisecond, 50*time.Millisecond)

	require.ErrorIs(t, err, ErrTimeout)
	assert.Contains(t, err.Error(), h.Hex())
}

func TestSendAppliesGasMultiplier(t *testing.T) {
	t.Parallel()

	key := testKey(1)
	chainID := big.NewInt(31337)
	to, err := client.HexToAddress("0x5FbDB2315678afecb367f032d93F642f64180aa3")
	require.NoError(t, err)

	fake := &mock.EVMClient{}
	fake.PendingNonceAtReturns(7, nil)
	fake.EstimateGasReturns(100000, nil)
	fake.SuggestGasFeesReturns(client.GasFees{
		MaxFeePerGas:         big.NewInt(20_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
	}, nil)
	fake.SendRawTransactionReturns(client.Hash{}, nil)

	s := &Submitter{Client: fake, Key: key, ChainID: chainID, GasMultiplier: 1.2}
	_, err = s.Send(context.Background(), &to, nil, []byte{0xde, 0xad, 0xbe, 0xef})
	require.NoError(t, err)

	require.Equal(t, 1, fake.SendRawTransactionCallCount())
	_, rawTx := fake.SendRawTransactionArgsForCall(0)

	// Re-derive the expected raw transaction (gas = 100000 * 1.2 = 120000) rather than decoding the
	// one the fake received: signing is deterministic, so the two must be byte-for-byte equal.
	wantTx := &client.DynamicFeeTx{
		ChainID:              chainID,
		Nonce:                7,
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		MaxFeePerGas:         big.NewInt(20_000_000_000),
		Gas:                  120000,
		To:                   &to,
		Data:                 []byte{0xde, 0xad, 0xbe, 0xef},
	}
	wantRaw, err := client.SignTx(wantTx, key)
	require.NoError(t, err)
	assert.Equal(t, wantRaw, rawTx)
}

func TestSendContractCreation(t *testing.T) {
	t.Parallel()

	key := testKey(1)
	chainID := big.NewInt(31337)
	deployData := []byte{0x60, 0x80, 0x60, 0x40}

	fake := &mock.EVMClient{}
	fake.PendingNonceAtReturns(0, nil)
	fake.EstimateGasReturns(50000, nil)
	fake.SuggestGasFeesReturns(client.GasFees{
		MaxFeePerGas:         big.NewInt(20_000_000_000),
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
	}, nil)
	fake.SendRawTransactionReturns(client.Hash{}, nil)

	s := &Submitter{Client: fake, Key: key, ChainID: chainID}
	_, err := s.Send(context.Background(), nil, nil, deployData)
	require.NoError(t, err)

	_, estMsg := fake.EstimateGasArgsForCall(0)
	assert.Nil(t, estMsg.To, "contract creation must estimate gas with a nil recipient")

	_, rawTx := fake.SendRawTransactionArgsForCall(0)
	wantTx := &client.DynamicFeeTx{
		ChainID:              chainID,
		Nonce:                0,
		MaxPriorityFeePerGas: big.NewInt(1_000_000_000),
		MaxFeePerGas:         big.NewInt(20_000_000_000),
		Gas:                  60000, // default multiplier (1.2) applied since none was set
		To:                   nil,
		Data:                 deployData,
	}
	wantRaw, err := client.SignTx(wantTx, key)
	require.NoError(t, err)
	assert.Equal(t, wantRaw, rawTx)
}
