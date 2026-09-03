/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
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
		metadata := make([][]byte, int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
		metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(pb.TxValidationCode_VALID)}
		block := &cb.Block{
			Header:   &cb.BlockHeader{Number: 1},
			Data:     &cb.BlockData{Data: [][]byte{[]byte("tx0")}},
			Metadata: &cb.BlockMetadata{Metadata: metadata},
		}

		code, err := validationCodeAt(block, 0)
		require.NoError(t, err)
		require.Equal(t, uint8(pb.TxValidationCode_VALID), code)
	})

	t.Run("nil metadata is rejected, not a panic", func(t *testing.T) {
		t.Parallel()
		block := &cb.Block{
			Header: &cb.BlockHeader{Number: 2},
			Data:   &cb.BlockData{Data: [][]byte{[]byte("tx0")}},
		}

		require.NotPanics(t, func() {
			_, err := validationCodeAt(block, 0)
			require.ErrorContains(t, err, "lacks transaction filter")
		})
	})

	t.Run("metadata missing transactions filter entry is rejected, not a panic", func(t *testing.T) {
		t.Parallel()
		metadata := make([][]byte, int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER))
		block := &cb.Block{
			Header:   &cb.BlockHeader{Number: 3},
			Data:     &cb.BlockData{Data: [][]byte{[]byte("tx0")}},
			Metadata: &cb.BlockMetadata{Metadata: metadata},
		}

		require.NotPanics(t, func() {
			_, err := validationCodeAt(block, 0)
			require.ErrorContains(t, err, "lacks transaction filter")
		})
	})

	t.Run("index beyond validation flags length is rejected, not a panic", func(t *testing.T) {
		t.Parallel()
		metadata := make([][]byte, int(cb.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
		metadata[cb.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(pb.TxValidationCode_VALID)}
		block := &cb.Block{
			Header: &cb.BlockHeader{Number: 4},
			// Two transactions in Data.Data, but the transactions filter only
			// covers one - the exact mismatch a malicious/misbehaving peer
			// can send.
			Data:     &cb.BlockData{Data: [][]byte{[]byte("tx0"), []byte("tx1")}},
			Metadata: &cb.BlockMetadata{Metadata: metadata},
		}

		require.NotPanics(t, func() {
			_, err := validationCodeAt(block, 1)
			require.ErrorContains(t, err, "out of range")
		})
	})
}

func TestServiceLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("NewService nil channelConfig", func(t *testing.T) {
		t.Parallel()
		svc, err := NewService(
			"testChannel",
			nil, // channelConfig
			"testNet",
			&mockLocalMembership{id: &mockSigningIdentity{}},
			&mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
			&mockServices{},
			&mockLedger{},
			&mockVault{},
			&mockTransactionManager{},
			func(ctx context.Context, block *cb.Block) (bool, error) { return false, nil },
			noop.NewTracerProvider(),
			nil,
			[]cb.HeaderType{cb.HeaderType_ENDORSER_TRANSACTION},
		)
		require.Error(t, err)
		require.Nil(t, svc)
	})

	t.Run("NewService succeeds", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t, testServiceOpts{})
		require.NotNil(t, svc)

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		err := svc.Start(ctx)
		require.NoError(t, err)

		svc.Stop()
		// Stop must be observable rather than merely not panicking, and the
		// background delivery must be finished before the subtest returns so it
		// cannot fail a later, unrelated test.
		<-svc.deliveryService.stop
		require.NoError(t, svc.deliveryService.stopError())

		// Stopping twice is a no-op, not a double close.
		svc.Stop()
	})
}

func TestScanBlockVariants(t *testing.T) {
	t.Run("Scan skips non-endorser and handles callback error", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		svc := newTestService(t, testServiceOpts{recvChan: recvChan})
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		envBytes := newValidEnvelopeBytes(t, cb.HeaderType_ENDORSER_TRANSACTION, "tx2")
		envBytesSkip := newValidEnvelopeBytes(t, cb.HeaderType_CONFIG, "tx-config")

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 14},
			Data:     &cb.BlockData{Data: [][]byte{envBytesSkip, envBytes}},
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, {uint8(pb.TxValidationCode_VALID), uint8(pb.TxValidationCode_VALID)}}},
		}}}

		err := svc.Scan(ctx, "txid", func(tx driver.ProcessedTransaction) (bool, error) {
			return false, errors.New("callback err")
		})
		require.ErrorContains(t, err, "callback err")
	})

	t.Run("Scan handles stop=true", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		svc := newTestService(t, testServiceOpts{recvChan: recvChan})
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		envBytes := newValidEnvelopeBytes(t, cb.HeaderType_ENDORSER_TRANSACTION, "tx2")

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 14},
			Data:     &cb.BlockData{Data: [][]byte{envBytes}},
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, {uint8(pb.TxValidationCode_VALID)}}},
		}}}

		err := svc.Scan(ctx, "txid", func(tx driver.ProcessedTransaction) (bool, error) {
			return true, nil
		})
		require.NoError(t, err)
	})

	t.Parallel()

	t.Run("ScanBlock fails immediately on canceled context", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t, testServiceOpts{})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		t.Cleanup(cancel)

		err := svc.ScanBlock(ctx, func(context.Context, *cb.Block) (bool, error) { return false, nil })
		if err != nil {
			require.ErrorContains(t, err, "context done")
		}
	})

	t.Run("ScanBlockFrom fails immediately on canceled context", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t, testServiceOpts{})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		t.Cleanup(cancel)

		err := svc.ScanBlockFrom(ctx, 10, func(context.Context, *cb.Block) (bool, error) { return false, nil })
		if err != nil {
			require.ErrorContains(t, err, "context done")
		}
	})

	t.Run("Scan fails immediately on canceled context", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		readChan := make(chan struct{})
		svc := newTestService(t, testServiceOpts{recvChan: recvChan, readChan: readChan})

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		t.Cleanup(cancel)

		err := svc.Scan(ctx, "tx1", func(tx driver.ProcessedTransaction) (bool, error) { return false, nil })
		if err != nil {
			require.ErrorContains(t, err, "context done")
		}
	})

	t.Run("Scan fails on unmarshal error with invalid block data", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		readChan := make(chan struct{})
		svc := newTestService(t, testServiceOpts{recvChan: recvChan, readChan: readChan})

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		go func() {
			<-readChan
			close(recvChan)
		}()

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 10},
			Data:     &cb.BlockData{Data: [][]byte{[]byte("invalid tx")}},
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, {uint8(pb.TxValidationCode_VALID)}}},
		}}}

		err := svc.Scan(ctx, "tx1", func(tx driver.ProcessedTransaction) (bool, error) { return false, nil })
		require.ErrorContains(t, err, "error unmarshalling")
	})

	t.Run("Scan processes valid block and returns false to continue", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		svc := newTestService(t, testServiceOpts{recvChan: recvChan})

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		envBytes := newValidEnvelopeBytes(t, cb.HeaderType_ENDORSER_TRANSACTION, "tx2")

		processed := make(chan struct{})
		go func() {
			<-processed
			close(recvChan)
			cancel()
		}()

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 11},
			Data:     &cb.BlockData{Data: [][]byte{envBytes}},
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, {uint8(pb.TxValidationCode_VALID)}}},
		}}}

		err := svc.Scan(ctx, "tx1", func(tx driver.ProcessedTransaction) (bool, error) {
			close(processed)
			return false, nil // return stop = false to reach the end of the loop
		})
		if err != nil {
			require.ErrorContains(t, err, "context done")
		}
	})

	t.Run("Scan processes valid block and returns true to stop", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		svc := newTestService(t, testServiceOpts{recvChan: recvChan})

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		envBytes := newValidEnvelopeBytes(t, cb.HeaderType_ENDORSER_TRANSACTION, "tx2")

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 16},
			Data:     &cb.BlockData{Data: [][]byte{envBytes}},
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, {uint8(pb.TxValidationCode_VALID)}}},
		}}}

		err := svc.ScanFromBlock(ctx, 10, func(tx driver.ProcessedTransaction) (bool, error) {
			return true, nil // return stop = true
		})
		require.NoError(t, err)
	})

	t.Run("ScanFromBlock handles callback error", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		svc := newTestService(t, testServiceOpts{recvChan: recvChan})

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		envBytes := newValidEnvelopeBytes(t, cb.HeaderType_ENDORSER_TRANSACTION, "tx2")

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 13},
			Data:     &cb.BlockData{Data: [][]byte{envBytes}},
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, {uint8(pb.TxValidationCode_VALID)}}},
		}}}
		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Status{Status: cb.Status_SUCCESS}}

		err := svc.ScanFromBlock(ctx, 10, func(tx driver.ProcessedTransaction) (bool, error) {
			return false, errors.New("callback error")
		})
		require.ErrorContains(t, err, "callback error")
	})

	t.Run("ScanFromBlock handles transaction manager error and skips non-endorser transactions", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		svc := newTestService(t, testServiceOpts{
			recvChan: recvChan,
			txMgr:    &mockTransactionManager{err: errors.New("tx mgr error")},
		})

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		envBytes := newValidEnvelopeBytes(t, cb.HeaderType_ENDORSER_TRANSACTION, "tx2")
		envBytesSkip := newValidEnvelopeBytes(t, cb.HeaderType_CONFIG, "tx-config")

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 14},
			Data:     &cb.BlockData{Data: [][]byte{envBytesSkip, envBytes}}, // skip first, fail on second
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, {uint8(pb.TxValidationCode_VALID), uint8(pb.TxValidationCode_VALID)}}},
		}}}

		err := svc.ScanFromBlock(ctx, 10, func(tx driver.ProcessedTransaction) (bool, error) {
			return false, nil
		})
		require.ErrorContains(t, err, "tx mgr error")
	})

	t.Run("ScanFromBlock handles unmarshal error for Envelope", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		svc := newTestService(t, testServiceOpts{recvChan: recvChan})

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 14},
			Data:     &cb.BlockData{Data: [][]byte{[]byte("invalid tx")}},
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, {uint8(pb.TxValidationCode_VALID)}}},
		}}}

		err := svc.ScanFromBlock(ctx, 10, func(tx driver.ProcessedTransaction) (bool, error) { return false, nil })
		require.ErrorContains(t, err, "error unmarshalling Envelope")
	})

	t.Run("ScanFromBlock handles missing transaction filter in metadata", func(t *testing.T) {
		t.Parallel()
		recvChan := make(chan *pb.DeliverResponse, 5)
		svc := newTestService(t, testServiceOpts{recvChan: recvChan})

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
			Header:   &cb.BlockHeader{Number: 15},
			Data:     &cb.BlockData{Data: [][]byte{[]byte("tx")}},
			Metadata: &cb.BlockMetadata{Metadata: [][]byte{}},
		}}}

		err := svc.ScanFromBlock(ctx, 10, func(tx driver.ProcessedTransaction) (bool, error) { return false, nil })
		require.ErrorContains(t, err, "metadata lacks transaction filter")
	})

	t.Run("ScanFromBlock connection failure propagates context error", func(t *testing.T) {
		t.Parallel()
		svc := newTestService(t, testServiceOpts{deliverErr: errors.New("deliver client error")})
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		// cancel synchronously so it fails when trying to sleep before reconnecting
		cancel()

		err := svc.ScanFromBlock(ctx, 10, func(tx driver.ProcessedTransaction) (bool, error) { return false, nil })
		if err != nil {
			require.ErrorContains(t, err, "context done")
		}
	})
}

func TestFakeVault(t *testing.T) {
	t.Parallel()
	v := &fakeVault{txID: "tx1", block: 10}
	txID, err := v.GetLastTxID(t.Context())
	require.NoError(t, err)
	require.Equal(t, "tx1", txID)

	block, err := v.GetLastBlock(t.Context())
	require.NoError(t, err)
	require.Equal(t, uint64(10), block)
}
