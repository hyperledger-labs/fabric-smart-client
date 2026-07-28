/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
)

// Regression coverage for a bug where Scan/ScanFromBlock indexed straight
// into block.Metadata.Metadata[TRANSACTIONS_FILTER][i] with no bounds
// checking. A malformed or malicious block delivered over the peer Deliver
// gRPC stream - one whose TRANSACTIONS_FILTER metadata entry is missing or
// shorter than block.Data.Data - crashed the whole process with an
// index-out-of-range panic, since nothing in the delivery call chain
// recovers panics. validationCodeAt is the extracted, bounds-checked
// replacement used by both Scan and ScanFromBlock.
func TestValidationCodeAt(t *testing.T) {
	t.Parallel()

	t.Run("returns validation code for a well-formed block", func(t *testing.T) {
		t.Parallel()
		metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
		metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(peer.TxValidationCode_VALID)}
		block := &common.Block{
			Header:   &common.BlockHeader{Number: 1},
			Data:     &common.BlockData{Data: [][]byte{[]byte("tx0")}},
			Metadata: &common.BlockMetadata{Metadata: metadata},
		}

		code, err := validationCodeAt(block, 0)
		require.NoError(t, err)
		require.Equal(t, uint8(peer.TxValidationCode_VALID), code)
	})

	t.Run("nil metadata is rejected, not a panic", func(t *testing.T) {
		t.Parallel()
		block := &common.Block{
			Header: &common.BlockHeader{Number: 2},
			Data:   &common.BlockData{Data: [][]byte{[]byte("tx0")}},
		}

		require.NotPanics(t, func() {
			_, err := validationCodeAt(block, 0)
			require.ErrorContains(t, err, "lacks transaction filter")
		})
	})

	t.Run("metadata missing transactions filter entry is rejected, not a panic", func(t *testing.T) {
		t.Parallel()
		metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER))
		block := &common.Block{
			Header:   &common.BlockHeader{Number: 3},
			Data:     &common.BlockData{Data: [][]byte{[]byte("tx0")}},
			Metadata: &common.BlockMetadata{Metadata: metadata},
		}

		require.NotPanics(t, func() {
			_, err := validationCodeAt(block, 0)
			require.ErrorContains(t, err, "lacks transaction filter")
		})
	})

	t.Run("index beyond validation flags length is rejected, not a panic", func(t *testing.T) {
		t.Parallel()
		metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
		metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(peer.TxValidationCode_VALID)}
		block := &common.Block{
			Header: &common.BlockHeader{Number: 4},
			// Two transactions in Data.Data, but the transactions filter only
			// covers one - the exact mismatch a malicious/misbehaving peer
			// can send.
			Data:     &common.BlockData{Data: [][]byte{[]byte("tx0"), []byte("tx1")}},
			Metadata: &common.BlockMetadata{Metadata: metadata},
		}

		require.NotPanics(t, func() {
			_, err := validationCodeAt(block, 1)
			require.ErrorContains(t, err, "out of range")
		})
	})
}
