/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"testing"
	"time"

	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// mockVault implements Vault for testing
type mockVault struct {
	lastTxID  string
	lastBlock uint64
	txIDErr   error
	blockErr  error
}

func (m *mockVault) GetLastTxID(_ context.Context) (string, error) {
	return m.lastTxID, m.txIDErr
}

func (m *mockVault) GetLastBlock(_ context.Context) (uint64, error) {
	return m.lastBlock, m.blockErr
}

// mockLedger implements driver.Ledger for testing
type mockLedger struct {
	blockNumber uint64
	blockErr    error
}

func (m *mockLedger) GetLedgerInfo() (*driver.LedgerInfo, error) { return nil, nil }
func (m *mockLedger) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	return nil, nil
}

func (m *mockLedger) GetBlockNumberByTxID(txID string) (uint64, error) {
	return m.blockNumber, m.blockErr
}
func (m *mockLedger) GetBlockByNumber(number uint64) (driver.Block, error) { return nil, nil }

// --- Tests for random.go ---

func TestGetRandomBytes(t *testing.T) {
	t.Parallel()
	t.Run("returns correct length", func(t *testing.T) {
		t.Parallel()
		b, err := GetRandomBytes(32)
		require.NoError(t, err)
		require.Len(t, b, 32)
	})

	t.Run("returns different values each call", func(t *testing.T) {
		t.Parallel()
		b1, err := GetRandomBytes(16)
		require.NoError(t, err)
		b2, err := GetRandomBytes(16)
		require.NoError(t, err)
		require.NotEqual(t, b1, b2)
	})

	t.Run("zero length returns empty slice", func(t *testing.T) {
		t.Parallel()
		b, err := GetRandomBytes(0)
		require.NoError(t, err)
		require.Empty(t, b)
	})
}

func TestGetRandomNonce(t *testing.T) {
	t.Parallel()
	nonce, err := GetRandomNonce()
	require.NoError(t, err)
	require.Len(t, nonce, NonceSize)
}

// --- Tests for SeekPosition ---

func TestSeekPosition(t *testing.T) {
	t.Parallel()
	t.Run("returns specified seek position", func(t *testing.T) {
		t.Parallel()
		pos := SeekPosition(42)
		require.NotNil(t, pos)
		specified, ok := pos.Type.(*ab.SeekPosition_Specified)
		require.True(t, ok)
		require.Equal(t, uint64(42), specified.Specified.Number)
	})

	t.Run("block zero returns seek position 0", func(t *testing.T) {
		t.Parallel()
		pos := SeekPosition(0)
		require.NotNil(t, pos)
		specified, ok := pos.Type.(*ab.SeekPosition_Specified)
		require.True(t, ok)
		require.Equal(t, uint64(0), specified.Specified.Number)
	})
}

func TestStartGenesis(t *testing.T) {
	t.Parallel()
	require.NotNil(t, StartGenesis)
	_, ok := StartGenesis.Type.(*ab.SeekPosition_Oldest)
	require.True(t, ok)
}

// --- Tests for New constructor ---

func TestNew_NilChannelConfig(t *testing.T) {
	t.Parallel()
	_, err := New(
		"test-network",
		nil, // nil channelConfig should error
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Second,
		1,
		nil,
		nil,
	)
	require.Error(t, err)
}

// --- Tests for GetStartPosition ---

func TestGetStartPosition(t *testing.T) {
	t.Parallel()

	t.Run("returns last block received if set", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			lastBlockReceived: 10,
			vault:             &mockVault{},
		}
		pos := d.GetStartPosition(t.Context())
		specified, ok := pos.Type.(*ab.SeekPosition_Specified)
		require.True(t, ok)
		require.Equal(t, uint64(10), specified.Specified.Number)
	})

	t.Run("returns vault last block if lastBlockReceived is 0", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			lastBlockReceived: 0,
			vault:             &mockVault{lastBlock: 5},
			Ledger:            &mockLedger{},
		}
		pos := d.GetStartPosition(t.Context())
		specified, ok := pos.Type.(*ab.SeekPosition_Specified)
		require.True(t, ok)
		require.Equal(t, uint64(5), specified.Specified.Number)
	})

	t.Run("returns genesis if vault errors on both block and txID", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			lastBlockReceived: 0,
			vault: &mockVault{
				blockErr: errors.New("block error"),
				txIDErr:  errors.New("txID error"),
			},
			Ledger: &mockLedger{},
		}
		pos := d.GetStartPosition(t.Context())
		_, ok := pos.Type.(*ab.SeekPosition_Oldest)
		require.True(t, ok)
	})

	t.Run("returns genesis if txID is empty", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			lastBlockReceived: 0,
			vault: &mockVault{
				blockErr: errors.New("block error"),
				lastTxID: "",
			},
			Ledger: &mockLedger{},
		}
		pos := d.GetStartPosition(t.Context())
		_, ok := pos.Type.(*ab.SeekPosition_Oldest)
		require.True(t, ok)
	})

	t.Run("returns block number from ledger for valid txID", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			lastBlockReceived: 0,
			vault: &mockVault{
				blockErr: errors.New("block error"),
				lastTxID: "valid-tx-id",
			},
			Ledger: &mockLedger{blockNumber: 7},
		}
		pos := d.GetStartPosition(t.Context())
		specified, ok := pos.Type.(*ab.SeekPosition_Specified)
		require.True(t, ok)
		require.Equal(t, uint64(7), specified.Specified.Number)
	})

	t.Run("returns genesis if ledger fails to get block by txID", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			lastBlockReceived: 0,
			vault: &mockVault{
				blockErr: errors.New("block error"),
				lastTxID: "valid-tx-id",
			},
			Ledger: &mockLedger{blockErr: errors.New("ledger error")},
		}
		pos := d.GetStartPosition(t.Context())
		_, ok := pos.Type.(*ab.SeekPosition_Oldest)
		require.True(t, ok)
	})
}

// --- Tests for processedTransaction ---

func TestProcessedTransaction(t *testing.T) {
	t.Parallel()
	pt := &processedTransaction{
		txID:    "tx-abc",
		results: []byte("results"),
		vc:      0, // TxValidationCode_VALID = 0
		env:     []byte("envelope"),
	}

	t.Run("TxID returns correct ID", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "tx-abc", pt.TxID())
	})

	t.Run("Results returns correct results", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []byte("results"), pt.Results())
	})

	t.Run("IsValid returns true for valid tx", func(t *testing.T) {
		t.Parallel()
		require.True(t, pt.IsValid())
	})

	t.Run("IsValid returns false for invalid tx", func(t *testing.T) {
		t.Parallel()
		invalid := &processedTransaction{vc: 1}
		require.False(t, invalid.IsValid())
	})

	t.Run("Envelope returns correct envelope", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, []byte("envelope"), pt.Envelope())
	})

	t.Run("ValidationCode returns correct code", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, int32(0), pt.ValidationCode())
	})
}
