/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
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

// --- Tests for handleBlockResponse ---
//
// Regression coverage for a bug where runReceiver's nil-block guard detected
// a malformed *pb.DeliverResponse_Block (e.g. Block.Header == nil, which a
// misbehaving or malicious peer can send over the Deliver gRPC stream) and
// reset connection state, but fell through unconditionally into
// r.Block.Header.Number afterwards, nil-pointer-panicking. Since runReceiver
// runs in a bare goroutine with no panic recovery anywhere in the call
// chain, this crashed the whole process (DoS). The fix, and what this test
// exercises directly, is that a malformed block must be rejected (returning
// false) without touching any of its nil fields.
func TestHandleBlockResponse(t *testing.T) {
	t.Parallel()

	newSpan := func(t *testing.T) trace.Span {
		t.Helper()
		_, span := noop.NewTracerProvider().Tracer("test").Start(t.Context(), "span")
		return span
	}

	t.Run("nil block is rejected, not dereferenced", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{client: &mockPeerClient{}}
		ch := make(chan blockResponse, 1)
		ok := d.handleBlockResponse(t.Context(), newSpan(t), &pb.DeliverResponse_Block{Block: nil}, ch, time.Millisecond)
		require.False(t, ok)
		require.Empty(t, ch)
	})

	t.Run("block with nil header is rejected, not dereferenced", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{client: &mockPeerClient{}}
		ch := make(chan blockResponse, 1)
		malformed := &cb.Block{
			Data:     &cb.BlockData{},
			Header:   nil,
			Metadata: &cb.BlockMetadata{},
		}
		ok := d.handleBlockResponse(t.Context(), newSpan(t), &pb.DeliverResponse_Block{Block: malformed}, ch, time.Millisecond)
		require.False(t, ok)
		require.Empty(t, ch)
	})

	t.Run("block with nil data is rejected, not dereferenced", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{client: &mockPeerClient{}}
		ch := make(chan blockResponse, 1)
		malformed := &cb.Block{
			Data:     nil,
			Header:   &cb.BlockHeader{Number: 1},
			Metadata: &cb.BlockMetadata{},
		}
		ok := d.handleBlockResponse(t.Context(), newSpan(t), &pb.DeliverResponse_Block{Block: malformed}, ch, time.Millisecond)
		require.False(t, ok)
		require.Empty(t, ch)
	})

	t.Run("block with nil metadata is rejected, not dereferenced", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{client: &mockPeerClient{}}
		ch := make(chan blockResponse, 1)
		malformed := &cb.Block{
			Data:     &cb.BlockData{},
			Header:   &cb.BlockHeader{Number: 1},
			Metadata: nil,
		}
		ok := d.handleBlockResponse(t.Context(), newSpan(t), &pb.DeliverResponse_Block{Block: malformed}, ch, time.Millisecond)
		require.False(t, ok)
		require.Empty(t, ch)
	})

	t.Run("well-formed block is pushed to channel", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{client: &mockPeerClient{}}
		ch := make(chan blockResponse, 1)
		good := &cb.Block{
			Data:     &cb.BlockData{},
			Header:   &cb.BlockHeader{Number: 42},
			Metadata: &cb.BlockMetadata{},
		}
		ok := d.handleBlockResponse(t.Context(), newSpan(t), &pb.DeliverResponse_Block{Block: good}, ch, time.Millisecond)
		require.True(t, ok)
		require.Len(t, ch, 1)
		received := <-ch
		require.Equal(t, uint64(42), received.block.Header.Number)
		require.Equal(t, uint64(42), d.lastBlockReceived)
	})
}

func TestDeliveryLifecycle(t *testing.T) {
	t.Parallel()

	tracerProvider := noop.NewTracerProvider()

	t.Run("Start, Stop, and Run", func(t *testing.T) {
		t.Parallel()

		d := &Delivery{
			bufferSize:    1,
			stop:          make(chan error),
			tracer:        tracerProvider.Tracer("test"),
			channelConfig: &mockChannelConfig{},
			ConfigService: &mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
			Services:      &mockServices{err: errors.New("err")},
		}

		ctx, cancel := context.WithCancel(t.Context())
		d.Start(ctx)

		go func() {
			time.Sleep(10 * time.Millisecond) // Give it time to start
			cancel()
		}()

		err := d.untilStop()
		require.ErrorContains(t, err, "context done")
	})

	t.Run("readBlocks stops on callback error", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			bufferSize: 1,
			stop:       make(chan error, 1),
			callback: func(ctx context.Context, block *cb.Block) (bool, error) {
				return false, errors.New("callback error")
			},
		}
		ch := make(chan blockResponse, 1)
		ch <- blockResponse{block: &cb.Block{Header: &cb.BlockHeader{}}}

		d.readBlocks(ch)
		err := <-d.stop
		require.ErrorContains(t, err, "callback error")
	})

	t.Run("readBlocks stops on stop true", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			bufferSize: 1,
			stop:       make(chan error, 1),
			callback: func(ctx context.Context, block *cb.Block) (bool, error) {
				return true, nil
			},
		}
		ch := make(chan blockResponse, 1)
		ch <- blockResponse{block: &cb.Block{Header: &cb.BlockHeader{}}}

		d.readBlocks(ch)
		err := <-d.stop
		require.NoError(t, err)
	})
}

func TestDeliveryRunNilCtx(t *testing.T) {
	t.Parallel()
	d := &Delivery{
		stop:          make(chan error, 1),
		bufferSize:    1,
		NetworkName:   "testNet",
		channel:       "testChannel",
		client:        &mockPeerClient{},
		tracer:        noop.NewTracerProvider().Tracer("test"),
		channelConfig: &mockChannelConfig{},
		ConfigService: &mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
		Services:      &mockServices{err: errors.New("err")},
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		d.stop <- errors.New("done")
		close(d.stop)
	}()
	err := d.Run(context.TODO())
	require.ErrorContains(t, err, "done")
}

func TestDeliveryConnect(t *testing.T) {
	t.Parallel()

	t.Run("NewPeerClient fails", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			ConfigService: &mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
			Services:      &mockServices{err: errors.New("peer client error")},
			NetworkName:   "testNet",
			channel:       "testChannel",
		}

		stream, cancel, err := d.connect(t.Context())
		require.ErrorContains(t, err, "failed creating peer client")
		require.Nil(t, stream)
		require.Nil(t, cancel)
	})

	t.Run("NewDeliverClient succeeds but NewDeliver fails", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			ConfigService: &mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
			Services: &mockServices{
				peerClient: &mockPeerClient{
					deliverErr: errors.New("deliver err"),
				},
			},
			NetworkName: "testNet",
			channel:     "testChannel",
		}

		stream, cancel, err := d.connect(t.Context())
		require.ErrorContains(t, err, "failed to get delivery stream")
		require.Nil(t, stream)
		if cancel != nil {
			cancel() // though usually cancel is called inside connect when error happens
		}
	})

	t.Run("DeliverSend fails", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			ConfigService: &mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
			Services: &mockServices{
				peerClient: &mockPeerClient{
					deliverCli: &mockDeliverClientRPC{
						stream: &mockDeliverStreamDelTest{sendErr: errors.New("send error")},
					},
				},
			},
			LocalMembership: &mockLocalMembership{
				id: &mockSigningIdentity{},
			},
			vault:       &mockVault{},
			NetworkName: "testNet",
			channel:     "testChannel",
		}

		d.Ledger = &mockLedger{blockNumber: 10}

		stream, cancel, err := d.connect(t.Context())
		require.ErrorContains(t, err, "failed sending seek envelope")
		require.Nil(t, stream)
		if cancel != nil {
			cancel()
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			ConfigService: &mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
			Services: &mockServices{
				peerClient: &mockPeerClient{
					deliverCli: &mockDeliverClientRPC{
						stream: &mockDeliverStreamDelTest{},
					},
				},
			},
			LocalMembership: &mockLocalMembership{
				id: &mockSigningIdentity{},
			},
			vault:       &mockVault{},
			NetworkName: "testNet",
			channel:     "testChannel",
		}
		d.Ledger = &mockLedger{blockNumber: 10}

		stream, cancel, err := d.connect(t.Context())
		require.NoError(t, err)
		require.NotNil(t, stream)
		if cancel != nil {
			cancel()
		}
	})
}

func TestRunReceiver(t *testing.T) {
	t.Parallel()
	tracerProvider := noop.NewTracerProvider()

	t.Run("context already done", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			tracer:        tracerProvider.Tracer("test"),
			stop:          make(chan error, 1),
			ConfigService: &mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
			Services:      &mockServices{err: errors.New("err")},
			channelConfig: &mockChannelConfig{},
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel() // already done
		ch := make(chan blockResponse, 1)

		d.runReceiver(ctx, ch)
		// Should exit quickly
		// it might consume the context done error itself and leave the channel empty/closed
		<-d.stop
	})

	t.Run("nil ctx", func(t *testing.T) {
		t.Parallel()
		d := &Delivery{
			NetworkName: "testNet",
			channel:     "testChannel",
			client:      &mockPeerClient{},
		}
		require.NotPanics(t, func() {
			d.runReceiver(context.TODO(), nil)
		})
	})
}

func TestRunReceiverResponses(t *testing.T) {
	t.Parallel()
	recvChan := make(chan *pb.DeliverResponse, 5)
	recvErrChan := make(chan error, 5)
	readChan := make(chan struct{}, 5)

	d := &Delivery{
		ConfigService: &mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
		Services: &mockServices{
			peerClient: &mockPeerClient{
				deliverCli: &mockDeliverClientRPC{
					stream: &mockDeliverStreamDelTest{
						recvChan:    recvChan,
						recvErrChan: recvErrChan,
						readChan:    readChan,
					},
				},
			},
		},
		LocalMembership: &mockLocalMembership{
			id: &mockSigningIdentity{},
		},
		vault:         &mockVault{},
		NetworkName:   "testNet",
		channel:       "testChannel",
		callback:      func(ctx context.Context, block *cb.Block) (bool, error) { return false, nil },
		channelConfig: &mockChannelConfig{},
		tracer:        noop.NewTracerProvider().Tracer("test"),
		client:        &mockPeerClient{},
		stop:          make(chan error, 1),
	}
	d.Ledger = &mockLedger{blockNumber: 10}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	ch := make(chan blockResponse, 5)

	recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Status{Status: cb.Status_SUCCESS}}
	recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Status{Status: cb.Status_NOT_FOUND}}
	recvChan <- &pb.DeliverResponse{Type: &pb.DeliverResponse_Block{Block: &cb.Block{
		Header:   &cb.BlockHeader{Number: 10},
		Data:     &cb.BlockData{},
		Metadata: &cb.BlockMetadata{Metadata: [][]byte{nil, nil, nil}},
	}}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		d.runReceiver(ctx, ch)
	}()

	// wait for 3 messages to be read
	for range 3 {
		select {
		case <-readChan:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}

	// Wait for the block to be processed and sent to ch (only Block gets sent to ch)
	select {
	case resp := <-ch:
		require.Equal(t, uint64(10), resp.block.Header.Number)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	close(recvChan)
	cancel()
	<-done
}
