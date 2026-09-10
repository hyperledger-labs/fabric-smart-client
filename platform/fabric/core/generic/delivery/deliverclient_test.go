/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
)

// fakeDeliverFiltered implements DeliverFiltered by replaying a fixed
// sequence of responses, one per Recv call.
type fakeDeliverFiltered struct {
	responses []*pb.DeliverResponse
	pos       int
}

func (*fakeDeliverFiltered) Send(*cb.Envelope) error { return nil }
func (*fakeDeliverFiltered) CloseSend() error        { return nil }

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

func TestNewDeliverClient(t *testing.T) {
	t.Parallel()
	mpc := &mockPeerClient{}
	client, err := NewDeliverClient(mpc)
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestNewDeliver(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mpc := &mockPeerClient{
			deliverCli: &mockDeliverClientRPC{stream: &mockDeliverStream{}},
		}
		client, _ := NewDeliverClient(mpc)
		stream, err := client.NewDeliver(t.Context())
		require.NoError(t, err)
		require.NotNil(t, stream)
	})

	t.Run("DeliverClient fails", func(t *testing.T) {
		t.Parallel()
		mpc := &mockPeerClient{
			deliverErr: errors.New("deliver client error"),
		}
		client, _ := NewDeliverClient(mpc)
		stream, err := client.NewDeliver(t.Context())
		require.ErrorContains(t, err, "failed to create deliver client for peer")
		require.Nil(t, stream)
	})

	t.Run("Deliver fails", func(t *testing.T) {
		t.Parallel()
		mpc := &mockPeerClient{
			deliverCli: &mockDeliverClientRPC{streamErr: errors.New("rpc error")},
		}
		client, _ := NewDeliverClient(mpc)
		stream, err := client.NewDeliver(t.Context())
		require.ErrorContains(t, err, "failed to new a deliver filtered")
		require.Nil(t, stream)
	})
}

func TestNewDeliverFiltered(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mpc := &mockPeerClient{
			deliverCli: &mockDeliverClientRPC{filtered: &mockDeliverFiltered{}},
		}
		client, _ := NewDeliverClient(mpc)
		stream, err := client.NewDeliverFiltered(t.Context())
		require.NoError(t, err)
		require.NotNil(t, stream)
	})

	t.Run("DeliverClient fails", func(t *testing.T) {
		t.Parallel()
		mpc := &mockPeerClient{
			deliverErr: errors.New("deliver client error"),
		}
		client, _ := NewDeliverClient(mpc)
		stream, err := client.NewDeliverFiltered(t.Context())
		require.ErrorContains(t, err, "failed to create deliver client for peer")
		require.Nil(t, stream)
	})

	t.Run("DeliverFiltered fails", func(t *testing.T) {
		t.Parallel()
		mpc := &mockPeerClient{
			deliverCli: &mockDeliverClientRPC{filteredErr: errors.New("rpc error")},
		}
		client, _ := NewDeliverClient(mpc)
		stream, err := client.NewDeliverFiltered(t.Context())
		require.ErrorContains(t, err, "failed to new a deliver filtered")
		require.Nil(t, stream)
	})
}

func TestCertificate(t *testing.T) {
	t.Parallel()
	cert := tls.Certificate{Certificate: [][]byte{[]byte("cert")}}
	mpc := &mockPeerClient{cert: cert}
	client, _ := NewDeliverClient(mpc)
	c := client.Certificate()
	require.NotNil(t, c)
	require.Equal(t, cert.Certificate, c.Certificate)
}

func TestCreateDeliverEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		cert := &tls.Certificate{}
		start := &ab.SeekPosition{}
		id := &mockSigningIdentity{}
		env, err := CreateDeliverEnvelope("mychannel", id, cert, start)
		require.NoError(t, err)
		require.NotNil(t, env)
		require.NotNil(t, env.Payload)
		require.NotNil(t, env.Signature)

		payload := &cb.Payload{}
		err = proto.Unmarshal(env.Payload, payload)
		require.NoError(t, err)

		chdr := &cb.ChannelHeader{}
		err = proto.Unmarshal(payload.Header.ChannelHeader, chdr)
		require.NoError(t, err)
		require.Equal(t, "mychannel", chdr.ChannelId)
	})

	t.Run("serialize fails", func(t *testing.T) {
		t.Parallel()
		cert := &tls.Certificate{}
		start := &ab.SeekPosition{}
		id := &mockSigningIdentity{serializeErr: errors.New("serialize error")}
		env, err := CreateDeliverEnvelope("mychannel", id, cert, start)
		require.ErrorContains(t, err, "serialize error")
		require.Nil(t, env)
	})

	t.Run("sign fails", func(t *testing.T) {
		t.Parallel()
		cert := &tls.Certificate{}
		start := &ab.SeekPosition{}
		id := &mockSigningIdentity{signErr: errors.New("sign error")}
		env, err := CreateDeliverEnvelope("mychannel", id, cert, start)
		require.ErrorContains(t, err, "sign error")
		require.Nil(t, env)
	})
}

func TestDeliverWaitForResponse(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		eventCh := make(chan TxEvent, 1)
		eventCh <- TxEvent{TxID: "tx1", Committed: true, Block: 10, IndexInBlock: 5}

		committed, block, idx, err := DeliverWaitForResponse(t.Context(), eventCh, "tx1")
		require.NoError(t, err)
		require.True(t, committed)
		require.Equal(t, uint64(10), block)
		require.Equal(t, 5, idx)
	})

	t.Run("txid mismatch", func(t *testing.T) {
		t.Parallel()
		eventCh := make(chan TxEvent, 1)
		eventCh <- TxEvent{TxID: "tx2"}

		committed, _, _, err := DeliverWaitForResponse(t.Context(), eventCh, "tx1")
		require.ErrorContains(t, err, "no event received for txid tx1")
		require.False(t, committed)
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		eventCh := make(chan TxEvent)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		t.Cleanup(cancel)

		committed, _, _, err := DeliverWaitForResponse(ctx, eventCh, "tx1")
		require.ErrorContains(t, err, "timed out waiting for committing txid [tx1]")
		require.False(t, committed)
	})
}

func TestDeliverSend(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		stream := &mockDeliverStream{}
		err := DeliverSend(stream, &cb.Envelope{})
		require.NoError(t, err)
	})

	t.Run("send fails", func(t *testing.T) {
		t.Parallel()
		stream := &mockDeliverStream{sendErr: errors.New("send error")}
		err := DeliverSend(stream, &cb.Envelope{})
		require.ErrorContains(t, err, "send error")
	})

	t.Run("close send fails", func(t *testing.T) {
		t.Parallel()
		stream := &mockDeliverStream{closeSendErr: errors.New("close error")}
		err := DeliverSend(stream, &cb.Envelope{})
		require.NoError(t, err) // DeliverSend returns the Send error, not the CloseSend error, but logs the CloseSend error.
	})
}

func TestDeliverReceive(t *testing.T) {
	t.Parallel()

	t.Run("Recv error", func(t *testing.T) {
		t.Parallel()
		df := &mockDeliverStream{recvErr: errors.New("recv error")}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.ErrorContains(t, err, "recv error")
		event := <-ch
		require.ErrorContains(t, event.Err, "recv error")
	})

	t.Run("FilteredBlock VALID", func(t *testing.T) {
		t.Parallel()
		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_FilteredBlock{
					FilteredBlock: &pb.FilteredBlock{
						Number: 10,
						FilteredTransactions: []*pb.FilteredTransaction{
							{Txid: "tx1", TxValidationCode: pb.TxValidationCode_VALID},
						},
					},
				},
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.NoError(t, err)
		event := <-ch
		require.True(t, event.Committed)
		require.Equal(t, uint64(10), event.Block)
		require.Equal(t, 0, event.IndexInBlock)
	})

	t.Run("FilteredBlock INVALID", func(t *testing.T) {
		t.Parallel()
		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_FilteredBlock{
					FilteredBlock: &pb.FilteredBlock{
						Number: 10,
						FilteredTransactions: []*pb.FilteredTransaction{
							{Txid: "tx1", TxValidationCode: pb.TxValidationCode_MVCC_READ_CONFLICT},
						},
					},
				},
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.ErrorContains(t, err, "status is not valid: MVCC_READ_CONFLICT")
	})

	t.Run("DeliverResponse_Status", func(t *testing.T) {
		t.Parallel()
		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_Status{
					Status: cb.Status_SUCCESS,
				},
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.ErrorContains(t, err, "deliver completed with status (SUCCESS) before txid tx1 received from peer peer0")
	})

	t.Run("DeliverResponse unexpected", func(t *testing.T) {
		t.Parallel()
		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: nil,
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.ErrorContains(t, err, "received unexpected response type")
	})

	t.Run("DeliverResponse_Block VALID", func(t *testing.T) {
		t.Parallel()
		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_Block{
					Block: blockWithTx(11, []string{"tx1"}, pb.TxValidationCode_VALID),
				},
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.NoError(t, err)
		event := <-ch
		require.True(t, event.Committed)
		require.Equal(t, uint64(11), event.Block)
		require.Equal(t, 0, event.IndexInBlock)
	})

	// A full block carries its verdicts in TRANSACTIONS_FILTER metadata rather
	// than inline, so matching the transaction ID only proves the transaction was
	// included in the block - Fabric includes rejected transactions too. The
	// verdict must be read before reporting the transaction as committed,
	// matching the FilteredBlock branch.
	t.Run("DeliverResponse_Block INVALID", func(t *testing.T) {
		t.Parallel()
		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_Block{
					Block: blockWithTx(11, []string{"tx1"}, pb.TxValidationCode_MVCC_READ_CONFLICT),
				},
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.ErrorContains(t, err, "status is not valid: MVCC_READ_CONFLICT")
		require.False(t, (<-ch).Committed, "an invalid transaction must not be reported as committed")
	})

	// Fail closed: a block whose validity information is absent or too short to
	// cover the matched transaction must not be reported as committed. Matches
	// how Service.Scan and Service.ScanFromBlock already treat validationCodeAt
	// errors. A malicious peer can send either shape.
	t.Run("DeliverResponse_Block missing validation metadata", func(t *testing.T) {
		t.Parallel()
		block := blockWithTx(11, []string{"tx1"}, pb.TxValidationCode_VALID)
		block.Metadata = nil

		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_Block{Block: block},
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.ErrorContains(t, err, "metadata lacks transaction filter")
		require.False(t, (<-ch).Committed, "a block without validity information must not be reported as committed")
	})

	t.Run("DeliverResponse_Block truncated validation metadata", func(t *testing.T) {
		t.Parallel()
		// Two transactions, but validity flags cover only the first, so the
		// matched transaction at index 1 has no verdict.
		block := blockWithTx(11, []string{"tx0", "tx1"}, pb.TxValidationCode_VALID)
		block.Metadata.Metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(pb.TxValidationCode_VALID)}

		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_Block{Block: block},
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.ErrorContains(t, err, "out of range")
		require.False(t, (<-ch).Committed, "a transaction with no verdict must not be reported as committed")
	})

	// The issue's stated expectation is that both branches agree. An invalid
	// transaction must produce the same error regardless of stream type.
	t.Run("both branches report an invalid transaction alike", func(t *testing.T) {
		t.Parallel()
		filtered := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_FilteredBlock{
					FilteredBlock: &pb.FilteredBlock{
						Number: 11,
						FilteredTransactions: []*pb.FilteredTransaction{
							{Txid: "tx1", TxValidationCode: pb.TxValidationCode_MVCC_READ_CONFLICT},
						},
					},
				},
			},
		}
		full := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_Block{
					Block: blockWithTx(11, []string{"tx1"}, pb.TxValidationCode_MVCC_READ_CONFLICT),
				},
			},
		}

		filteredErr := DeliverReceive(filtered, "peer0", "tx1", make(chan TxEvent, 1))
		fullErr := DeliverReceive(full, "peer0", "tx1", make(chan TxEvent, 1))

		require.Error(t, filteredErr)
		require.Error(t, fullErr)
		require.Equal(t, filteredErr.Error(), fullErr.Error(),
			"the two stream types must report an invalid transaction identically")
	})

	t.Run("DeliverResponse_Block invalid envelope", func(t *testing.T) {
		t.Parallel()
		df := &mockDeliverStream{
			recvResp: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_Block{
					Block: &cb.Block{
						Header: &cb.BlockHeader{Number: 11},
						Data: &cb.BlockData{
							Data: [][]byte{[]byte("garbage")},
						},
					},
				},
			},
		}
		ch := make(chan TxEvent, 1)
		err := DeliverReceive(df, "peer0", "tx1", ch)
		require.ErrorContains(t, err, "error parsing transaction")
	})
}

// blockWithTx builds a full block containing the given transaction IDs, with a
// TRANSACTIONS_FILTER metadata entry marking every one of them with code. The
// filter lives at index BlockMetadataIndex_TRANSACTIONS_FILTER (2), so the
// earlier entries are padding.
func blockWithTx(number uint64, txIDs []string, code pb.TxValidationCode) *cb.Block {
	data := make([][]byte, 0, len(txIDs))
	flags := make([]byte, 0, len(txIDs))
	for _, txID := range txIDs {
		chdrBytes, err := proto.Marshal(&cb.ChannelHeader{TxId: txID})
		if err != nil {
			panic(err)
		}
		payloadBytes, err := proto.Marshal(&cb.Payload{Header: &cb.Header{ChannelHeader: chdrBytes}})
		if err != nil {
			panic(err)
		}
		envBytes, err := proto.Marshal(&cb.Envelope{Payload: payloadBytes})
		if err != nil {
			panic(err)
		}
		data = append(data, envBytes)
		flags = append(flags, uint8(code))
	}

	metadata := make([][]byte, cb.BlockMetadataIndex_TRANSACTIONS_FILTER+1)
	metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER] = flags

	return &cb.Block{
		Header:   &cb.BlockHeader{Number: number},
		Data:     &cb.BlockData{Data: data},
		Metadata: &cb.BlockMetadata{Metadata: metadata},
	}
}
