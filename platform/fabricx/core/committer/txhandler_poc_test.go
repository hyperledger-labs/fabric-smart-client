/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	commoncommitter "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
)

// TestHandleFabricxTransactionShortTransactionsFilterReturnsError proves the
// fix: HandleFabricxTransaction now bounds-checks the TRANSACTIONS_FILTER
// entry itself (blkMetadata.Metadata[statusIdx], a byte slice with one status
// byte per transaction in the block) against tx.TxNum, instead of only
// bounds-checking the outer block-metadata slice.
//
// A compromised/malicious committer or notifier delivering a block/metadata
// pair whose TRANSACTIONS_FILTER entry is shorter than the actual number of
// transactions -- or simply reporting a tx.TxNum beyond that entry's length
// -- used to cause an index-out-of-range panic that crashed the goroutine
// processing the block (platform/fabric/core/generic/committer/committer.go's
// commitTxs, run inside an errgroup per block), i.e. a remotely triggerable
// DoS against the FSC client process. It must now return an error instead.
func TestHandleFabricxTransactionShortTransactionsFilterReturnsError(t *testing.T) {
	h := &handler{committer: &commoncommitter.Committer{}}

	// TRANSACTIONS_FILTER (index 2) is present but empty, i.e. it reports
	// status for zero transactions, while the committer still asks us to
	// handle tx.TxNum == 0.
	blkMetadata := &cb.BlockMetadata{
		Metadata: [][]byte{
			{}, // SIGNATURES
			{}, // LAST_CONFIG
			{}, // TRANSACTIONS_FILTER -- empty, shorter than the block's tx count
		},
	}

	tx := commoncommitter.CommitTx{
		TxNum: 0,
		TxID:  "victim-tx",
	}

	require.NotPanics(t, func() {
		event, err := h.HandleFabricxTransaction(context.Background(), blkMetadata, tx)
		require.Error(t, err)
		require.Nil(t, event)
		require.Contains(t, err.Error(), "transaction filter has no entry")
	})
}

// TestHandleFabricxTransactionShortMetadataSliceReturnsError proves the fix
// for the second, off-by-one gap: the original outer check
// (len(blkMetadata.Metadata) < statusIdx, with statusIdx == 2) only guaranteed
// indices 0 and 1 existed, not index 2 (TRANSACTIONS_FILTER) itself -- a
// block metadata slice with exactly statusIdx (2) entries would still panic
// on Metadata[statusIdx]. The check must require statusIdx+1 entries.
func TestHandleFabricxTransactionShortMetadataSliceReturnsError(t *testing.T) {
	h := &handler{committer: &commoncommitter.Committer{}}

	// Only 2 metadata entries -- TRANSACTIONS_FILTER (index 2) is missing
	// entirely, not just short.
	blkMetadata := &cb.BlockMetadata{
		Metadata: [][]byte{
			{}, // SIGNATURES
			{}, // LAST_CONFIG
		},
	}

	tx := commoncommitter.CommitTx{
		TxNum: 0,
		TxID:  "victim-tx",
	}

	require.NotPanics(t, func() {
		event, err := h.HandleFabricxTransaction(context.Background(), blkMetadata, tx)
		require.Error(t, err)
		require.Nil(t, event)
		require.Contains(t, err.Error(), "lacks transaction filter")
	})
}
