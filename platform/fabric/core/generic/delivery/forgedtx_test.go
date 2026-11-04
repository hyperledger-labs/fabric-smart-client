/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// envelopeCode is an envelope of header type typ flagged with code.
type envelopeCode struct {
	typ  cb.HeaderType
	code pb.TxValidationCode
}

// endorser returns an envelopeCode of an endorser transaction for each code.
func endorser(codes ...pb.TxValidationCode) []envelopeCode {
	envs := make([]envelopeCode, len(codes))
	for i, code := range codes {
		envs[i] = envelopeCode{cb.HeaderType_ENDORSER_TRANSACTION, code}
	}
	return envs
}

// forgedBlock returns a block numbered 7 holding an envelope with txID for each
// of envs, in order.
func forgedBlock(t *testing.T, txID string, envs ...envelopeCode) *cb.Block {
	t.Helper()
	flags := make([]byte, len(envs))
	data := make([][]byte, len(envs))
	for i, env := range envs {
		flags[i] = uint8(env.code)
		data[i] = newValidEnvelopeBytes(t, env.typ, txID)
	}
	return &cb.Block{
		Header:   &cb.BlockHeader{Number: 7},
		Data:     &cb.BlockData{Data: data},
		Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, flags}},
	}
}

// forgedResponses returns the block of forgedBlock for txID tx1 in both shapes
// a Deliver stream can carry.
func forgedResponses(t *testing.T, envs ...envelopeCode) map[string]*pb.DeliverResponse {
	t.Helper()
	filtered := &pb.FilteredBlock{Number: 7}
	for _, env := range envs {
		filtered.FilteredTransactions = append(filtered.FilteredTransactions, &pb.FilteredTransaction{
			Txid: "tx1", Type: env.typ, TxValidationCode: env.code,
		})
	}
	return map[string]*pb.DeliverResponse{
		"Block":         {Type: &pb.DeliverResponse_Block{Block: forgedBlock(t, "tx1", envs...)}},
		"FilteredBlock": {Type: &pb.DeliverResponse_FilteredBlock{FilteredBlock: filtered}},
	}
}

// TestDeliverReceiveForgedEnvelope streams blocks holding several envelopes
// with one txID, in both block shapes, and checks that DeliverReceive reports
// the verdict of the valid endorser transaction, whatever the header type of
// the forgery ahead of it, while a block holding only invalid envelopes still
// reports the first one.
func TestDeliverReceiveForgedEnvelope(t *testing.T) {
	t.Parallel()
	const valid = pb.TxValidationCode_VALID
	honest := envelopeCode{cb.HeaderType_ENDORSER_TRANSACTION, valid}
	for name, tc := range map[string]struct {
		envs  []envelopeCode
		index int
	}{
		"forged then honest":         {endorser(pb.TxValidationCode_BAD_PROPOSAL_TXID, valid), 1},
		"forged message then honest": {[]envelopeCode{{cb.HeaderType_MESSAGE, pb.TxValidationCode_UNSUPPORTED_TX_PAYLOAD}, honest}, 1},
		"honest then duplicate":      {endorser(valid, pb.TxValidationCode_DUPLICATE_TXID), 0},
	} {
		for shape, resp := range forgedResponses(t, tc.envs...) {
			t.Run(name+"/"+shape, func(t *testing.T) {
				t.Parallel()
				ch := make(chan TxEvent, 1)
				require.NoError(t, DeliverReceive(&mockDeliverStream{recvResp: resp}, "peer0", "tx1", ch))
				event := <-ch
				assert.True(t, event.Committed)
				assert.Equal(t, uint64(7), event.Block)
				assert.Equal(t, tc.index, event.IndexInBlock)
			})
		}
	}
	for name, resp := range forgedResponses(t, endorser(pb.TxValidationCode_BAD_PROPOSAL_TXID, pb.TxValidationCode_MVCC_READ_CONFLICT)...) {
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
		codes, err := scan(t, forgedBlock(t, "tx2", endorser(forged, valid)...), 1)
		require.NoError(t, err)
		require.Equal(t, []pb.TxValidationCode{valid}, codes)
	})
	t.Run("invalid only", func(t *testing.T) {
		t.Parallel()
		codes, err := scan(t, forgedBlock(t, "tx2", endorser(forged, pb.TxValidationCode_MVCC_READ_CONFLICT)...), 2)
		require.NoError(t, err)
		require.Equal(t, []pb.TxValidationCode{forged, pb.TxValidationCode_MVCC_READ_CONFLICT}, codes)
	})
	t.Run("shadowed index beyond the flags", func(t *testing.T) {
		t.Parallel()
		block := forgedBlock(t, "tx2", endorser(valid, valid)...)
		block.Metadata.Metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(valid)}
		_, err := scan(t, block, 2)
		require.ErrorContains(t, err, "out of range for validation flags")
	})
}
