/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"errors"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
)

// fakeDeliverFiltered implements DeliverFiltered by replaying a fixed
// sequence of responses, one per Recv call.
type fakeDeliverFiltered struct {
	responses []*pb.DeliverResponse
	pos       int
}

func (f *fakeDeliverFiltered) Send(*cb.Envelope) error { return nil }
func (f *fakeDeliverFiltered) CloseSend() error        { return nil }

func (f *fakeDeliverFiltered) Recv() (*pb.DeliverResponse, error) {
	if f.pos >= len(f.responses) {
		return nil, errors.New("EOF")
	}
	r := f.responses[f.pos]
	f.pos++
	return r, nil
}

// Regression coverage for a bug where the *pb.DeliverResponse_Block case in
// DeliverReceive indexed straight into r.Block.Data.Data and
// r.Block.Header.Number with no nil-check, unlike the equivalent, already
// nil-checked path in handleBlockResponse (see TestHandleBlockResponse). The
// Block field of a DeliverResponse_Block, and its own Data/Header/Metadata
// submessages, can independently be nil after unmarshalling a
// well-formed-at-the-wire-level but malicious/malformed message from a
// misbehaving peer. DeliverReceive is invoked from a bare goroutine (see
// FabricFinality.IsFinal) with no panic recovery anywhere in the call chain,
// so a nil dereference here crashes the whole process (DoS).
func TestDeliverReceive_MalformedBlock(t *testing.T) {
	t.Parallel()

	t.Run("nil block is rejected, not dereferenced", func(t *testing.T) {
		t.Parallel()
		df := &fakeDeliverFiltered{responses: []*pb.DeliverResponse{
			{Type: &pb.DeliverResponse_Block{Block: nil}},
		}}
		eventCh := make(chan TxEvent, 1)

		require.NotPanics(t, func() {
			err := DeliverReceive(df, "peer0", "tx1", eventCh)
			require.Error(t, err)
			require.ErrorContains(t, err, "malformed block")
		})
	})

	t.Run("block with nil data is rejected, not dereferenced", func(t *testing.T) {
		t.Parallel()
		df := &fakeDeliverFiltered{responses: []*pb.DeliverResponse{
			{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
				Header: &cb.BlockHeader{Number: 1},
				Data:   nil,
			}}},
		}}
		eventCh := make(chan TxEvent, 1)

		require.NotPanics(t, func() {
			err := DeliverReceive(df, "peer0", "tx1", eventCh)
			require.Error(t, err)
			require.ErrorContains(t, err, "malformed block")
		})
	})

	t.Run("block with nil header is rejected, not dereferenced", func(t *testing.T) {
		t.Parallel()
		df := &fakeDeliverFiltered{responses: []*pb.DeliverResponse{
			{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
				Header: nil,
				Data:   &cb.BlockData{Data: [][]byte{[]byte("tx0")}},
			}}},
		}}
		eventCh := make(chan TxEvent, 1)

		require.NotPanics(t, func() {
			err := DeliverReceive(df, "peer0", "tx1", eventCh)
			require.Error(t, err)
			require.ErrorContains(t, err, "malformed block")
		})
	})
}
