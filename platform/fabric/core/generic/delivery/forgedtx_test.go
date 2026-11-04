/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// forgedBlock returns a block numbered 7 holding one endorser envelope with
// txID for each code, in order.
func forgedBlock(t *testing.T, txID string, codes ...pb.TxValidationCode) *cb.Block {
	t.Helper()
	env := newValidEnvelopeBytes(t, cb.HeaderType_ENDORSER_TRANSACTION, txID)
	flags := make([]byte, len(codes))
	data := make([][]byte, len(codes))
	for i, code := range codes {
		flags[i] = uint8(code)
		data[i] = env
	}
	return &cb.Block{
		Header:   &cb.BlockHeader{Number: 7},
		Data:     &cb.BlockData{Data: data},
		Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, flags}},
	}
}

// forgedResponses returns the block of forgedBlock for txID tx1 in both shapes
// a Deliver stream can carry.
func forgedResponses(t *testing.T, codes ...pb.TxValidationCode) map[string]*pb.DeliverResponse {
	t.Helper()
	filtered := &pb.FilteredBlock{Number: 7}
	for _, code := range codes {
		filtered.FilteredTransactions = append(filtered.FilteredTransactions, &pb.FilteredTransaction{
			Txid: "tx1", Type: cb.HeaderType_ENDORSER_TRANSACTION, TxValidationCode: code,
		})
	}
	return map[string]*pb.DeliverResponse{
		"Block":         {Type: &pb.DeliverResponse_Block{Block: forgedBlock(t, "tx1", codes...)}},
		"FilteredBlock": {Type: &pb.DeliverResponse_FilteredBlock{FilteredBlock: filtered}},
	}
}

// TestDeliverReceiveForgedEnvelope streams a block holding a forged envelope
// and the honest one with the same txID, in both block shapes, and checks that
// DeliverReceive reports the honest envelope's verdict, while a block holding
// only invalid envelopes still reports the first one.
func TestDeliverReceiveForgedEnvelope(t *testing.T) {
	t.Parallel()
	for name, resp := range forgedResponses(t, pb.TxValidationCode_BAD_PROPOSAL_TXID, pb.TxValidationCode_VALID) {
		t.Run("forged then honest/"+name, func(t *testing.T) {
			t.Parallel()
			ch := make(chan TxEvent, 1)
			require.NoError(t, DeliverReceive(&mockDeliverStream{recvResp: resp}, "peer0", "tx1", ch))
			event := <-ch
			require.True(t, event.Committed)
			require.Equal(t, uint64(7), event.Block)
			require.Equal(t, 1, event.IndexInBlock)
		})
	}
	for name, resp := range forgedResponses(t, pb.TxValidationCode_BAD_PROPOSAL_TXID, pb.TxValidationCode_MVCC_READ_CONFLICT) {
		t.Run("invalid only/"+name, func(t *testing.T) {
			t.Parallel()
			ch := make(chan TxEvent, 1)
			err := DeliverReceive(&mockDeliverStream{recvResp: resp}, "peer0", "tx1", ch)
			require.ErrorContains(t, err, "status is not valid: BAD_PROPOSAL_TXID")
			require.False(t, (<-ch).Committed)
		})
	}
}

// TestScanForgedEnvelope scans a block holding several envelopes with one txID
// and checks the validation codes handed to the callback: a forged envelope
// shadowed by the honest one is skipped, an invalid one without a valid twin is
// not, and a shadowed index beyond the validation flags still fails the scan.
func TestScanForgedEnvelope(t *testing.T) {
	t.Parallel()
	const (
		valid  = pb.TxValidationCode_VALID
		forged = pb.TxValidationCode_BAD_PROPOSAL_TXID
	)
	// scan delivers block and returns the codes of the first n transactions the
	// callback receives.
	scan := func(t *testing.T, block *cb.Block, n int) ([]pb.TxValidationCode, error) {
		t.Helper()
		recvChan := make(chan *pb.DeliverResponse, 1)
		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: block}}
		svc := newTestService(t, testServiceOpts{recvChan: recvChan})
		var codes []pb.TxValidationCode
		err := svc.ScanFromBlock(t.Context(), 7, func(tx driver.ProcessedTransaction) (bool, error) {
			codes = append(codes, pb.TxValidationCode(tx.ValidationCode()))
			return len(codes) == n, nil
		})
		return codes, err
	}

	t.Run("forged then honest", func(t *testing.T) {
		t.Parallel()
		codes, err := scan(t, forgedBlock(t, "tx2", forged, valid), 1)
		require.NoError(t, err)
		require.Equal(t, []pb.TxValidationCode{valid}, codes)
	})
	t.Run("invalid only", func(t *testing.T) {
		t.Parallel()
		codes, err := scan(t, forgedBlock(t, "tx2", forged, pb.TxValidationCode_MVCC_READ_CONFLICT), 2)
		require.NoError(t, err)
		require.Equal(t, []pb.TxValidationCode{forged, pb.TxValidationCode_MVCC_READ_CONFLICT}, codes)
	})
	t.Run("shadowed index beyond the flags", func(t *testing.T) {
		t.Parallel()
		block := forgedBlock(t, "tx2", valid, valid)
		block.Metadata.Metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(valid)}
		_, err := scan(t, block, 2)
		require.ErrorContains(t, err, "out of range for validation flags")
	})
}
