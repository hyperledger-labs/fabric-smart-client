/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"crypto/tls"
	"sync/atomic"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// --- Peer and deliver client mocks ---

type mockPeerClient struct {
	addr       string
	deliverCli pb.DeliverClient
	deliverErr error
	cert       tls.Certificate
}

func (m *mockPeerClient) Address() string                                    { return m.addr }
func (m *mockPeerClient) Certificate() tls.Certificate                       { return m.cert }
func (m *mockPeerClient) Close()                                             {}
func (m *mockPeerClient) EndorserClient() (pb.EndorserClient, error)         { return nil, nil }
func (m *mockPeerClient) DiscoveryClient() (services.DiscoveryClient, error) { return nil, nil }

// DeliverClient never returns a nil client with a nil error: a real peer client
// cannot do that, and NewDeliver would dereference the nil interface and take
// the whole test binary down with it. Tests that reach connect without
// configuring a deliver client get a clean failure instead.
func (m *mockPeerClient) DeliverClient() (pb.DeliverClient, error) {
	if m.deliverCli == nil && m.deliverErr == nil {
		return nil, errors.New("mockPeerClient: no deliver client configured")
	}
	return m.deliverCli, m.deliverErr
}

type mockDeliverClientRPC struct {
	stream      pb.Deliver_DeliverClient
	streamErr   error
	filtered    pb.Deliver_DeliverFilteredClient
	filteredErr error
}

func (m *mockDeliverClientRPC) Deliver(_ context.Context, _ ...googlegrpc.CallOption) (pb.Deliver_DeliverClient, error) {
	return m.stream, m.streamErr
}

func (m *mockDeliverClientRPC) DeliverFiltered(_ context.Context, _ ...googlegrpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
	return m.filtered, m.filteredErr
}

func (m *mockDeliverClientRPC) DeliverWithPrivateData(_ context.Context, _ ...googlegrpc.CallOption) (pb.Deliver_DeliverWithPrivateDataClient, error) {
	return nil, nil
}

// mockDeliverFiltered stands in for a filtered Deliver stream. Tests only ever
// use it as an opaque non-nil value, so the embedded interface is never called.
type mockDeliverFiltered struct {
	pb.Deliver_DeliverFilteredClient
}

// --- Identity mock ---

type mockSigningIdentity struct {
	serializeErr error
	signErr      error
}

func (m *mockSigningIdentity) Serialize() ([]byte, error) {
	if m.serializeErr != nil {
		return nil, m.serializeErr
	}
	return []byte("serialized"), nil
}

func (m *mockSigningIdentity) Sign(msg []byte) ([]byte, error) {
	if m.signErr != nil {
		return nil, m.signErr
	}
	return []byte("signature"), nil
}

// mockDeliverStream is a DeliverStream whose Recv works in one of two modes.
//
// Static mode (recvResp and/or recvErr set) returns the same result on every
// call, which suits the single-exchange DeliverSend/DeliverReceive tests.
// Scripted mode (recvChan set) serves queued responses in order, which suits
// the multi-step runReceiver and Scan tests; recvErrChan injects a transport
// error ahead of the queue, and readChan, if set, receives one value per
// response actually delivered so a test can wait for progress instead of
// sleeping.
//
// It embeds pb.Deliver_DeliverClient so it can also be assigned where a gRPC
// stream is expected. The embedded interface is nil, so calling any method
// beyond Send, Recv and CloseSend panics — no test does.
type mockDeliverStream struct {
	pb.Deliver_DeliverClient

	sendErr      error
	closeSendErr error

	// static mode
	recvResp *pb.DeliverResponse
	recvErr  error

	// scripted mode
	recvChan    chan *pb.DeliverResponse
	recvErrChan chan error
	readChan    chan struct{}
}

func (m *mockDeliverStream) Send(*cb.Envelope) error { return m.sendErr }
func (m *mockDeliverStream) CloseSend() error        { return m.closeSendErr }

func (m *mockDeliverStream) Recv() (*pb.DeliverResponse, error) {
	if m.recvErrChan != nil {
		select {
		case err := <-m.recvErrChan:
			if err != nil {
				return nil, err
			}
		default:
		}
	}

	if m.recvChan != nil {
		resp, ok := <-m.recvChan
		if !ok {
			return nil, errors.New("closed")
		}
		// Signal only for responses actually delivered. Signalling on a closed
		// recvChan too would let a caller that spins on the resulting error
		// fill readChan and then block here forever, hanging the test that is
		// waiting for the caller to return.
		if m.readChan != nil {
			m.readChan <- struct{}{}
		}
		return resp, nil
	}

	if m.recvResp != nil || m.recvErr != nil {
		return m.recvResp, m.recvErr
	}
	return nil, errors.New("mockDeliverStream: Recv not configured")
}

// --- Infrastructure mocks ---

type mockConfigService struct {
	driver.ConfigService
	peerConf *grpc.ConnectionConfig
}

func (m *mockConfigService) PickPeer(driver.PeerFunctionType) *grpc.ConnectionConfig {
	return m.peerConf
}

type mockServices struct {
	peerClient services.PeerClient
	err        error
}

func (m *mockServices) NewPeerClient(grpc.ConnectionConfig) (services.PeerClient, error) {
	return m.peerClient, m.err
}

// mockFlakyServices refuses the first failures calls to NewPeerClient and then
// hands out peerClient, so a test can drive the reconnect path. Every attempt is
// announced on attempts, if set, so a test can wait for progress instead of
// sleeping.
type mockFlakyServices struct {
	peerClient services.PeerClient
	failures   atomic.Int32
	attempts   chan struct{}
}

func (m *mockFlakyServices) NewPeerClient(grpc.ConnectionConfig) (services.PeerClient, error) {
	if m.attempts != nil {
		select {
		case m.attempts <- struct{}{}:
		default: // a test that stopped counting must not wedge the receiver
		}
	}
	if m.failures.Add(-1) >= 0 {
		return nil, errors.New("mockFlakyServices: connection refused")
	}
	return m.peerClient, nil
}

type mockLocalMembership struct {
	driver.LocalMembership
	id driver.SigningIdentity
}

func (m *mockLocalMembership) DefaultSigningIdentity() driver.SigningIdentity {
	return m.id
}

type mockChannelConfig struct {
	driver.ChannelConfig
}

func (m *mockChannelConfig) DeliverySleepAfterFailure() time.Duration { return 10 * time.Millisecond }

func (m *mockChannelConfig) CommitterWaitForEventTimeout() time.Duration {
	return 10 * time.Millisecond
}
func (m *mockChannelConfig) DeliveryBufferSize() int { return 1 }
func (m *mockChannelConfig) ID() string              { return "testChannel" }

// --- Transaction mocks ---

type mockTransactionManager struct {
	ptx driver.ProcessedTransaction
	err error
}

func (m *mockTransactionManager) ComputeTxID(*driver.TxIDComponents) string { return "" }
func (m *mockTransactionManager) NewEnvelope() driver.Envelope              { return nil }
func (m *mockTransactionManager) NewProposalResponseFromBytes([]byte) (driver.ProposalResponse, error) {
	return nil, nil
}

func (m *mockTransactionManager) NewTransaction(context.Context, driver.TransactionType, view.Identity, []byte, string, string, []byte) (driver.Transaction, error) {
	return nil, nil
}

func (m *mockTransactionManager) NewTransactionFromBytes(context.Context, string, []byte) (driver.Transaction, error) {
	return nil, nil
}

func (m *mockTransactionManager) NewTransactionFromEnvelopeBytes(context.Context, string, []byte) (driver.Transaction, error) {
	return nil, nil
}

func (m *mockTransactionManager) AddTransactionFactory(driver.TransactionType, driver.TransactionFactory) {
}

func (m *mockTransactionManager) NewProcessedTransactionFromEnvelopePayload([]byte) (driver.ProcessedTransaction, int32, error) {
	return nil, 0, nil
}

func (m *mockTransactionManager) NewProcessedTransaction([]byte) (driver.ProcessedTransaction, error) {
	return nil, nil
}

func (m *mockTransactionManager) NewProcessedTransactionFromEnvelopeRaw([]byte) (driver.ProcessedTransaction, error) {
	return m.ptx, m.err
}

type mockProcessedTx struct {
	txID    string
	results []byte
	env     []byte
}

func (m *mockProcessedTx) TxID() string          { return m.txID }
func (m *mockProcessedTx) Results() []byte       { return m.results }
func (m *mockProcessedTx) Envelope() []byte      { return m.env }
func (m *mockProcessedTx) ValidationCode() int32 { return 0 }
func (m *mockProcessedTx) IsValid() bool         { return true }

// --- Test helpers ---

// testServiceOpts configures newTestService. Zero values yield sensible defaults.
type testServiceOpts struct {
	recvChan   chan *pb.DeliverResponse
	readChan   chan struct{}
	deliverErr error
	txMgr      *mockTransactionManager
	ledger     *mockLedger
}

// newTestService creates a *Service with sensible defaults, failing the test on error.
func newTestService(t *testing.T, opts testServiceOpts) *Service {
	t.Helper()

	txMgr := opts.txMgr
	if txMgr == nil {
		txMgr = &mockTransactionManager{ptx: &mockProcessedTx{txID: "tx2"}}
	}

	ledger := opts.ledger
	if ledger == nil {
		ledger = &mockLedger{blockNumber: 10}
	}

	var pc *mockPeerClient
	switch {
	case opts.deliverErr != nil:
		pc = &mockPeerClient{deliverErr: opts.deliverErr}
	case opts.recvChan != nil:
		pc = &mockPeerClient{
			deliverCli: &mockDeliverClientRPC{
				stream: &mockDeliverStream{
					recvChan: opts.recvChan,
					readChan: opts.readChan,
				},
			},
		}
	default:
		pc = &mockPeerClient{}
	}

	svc, err := NewService(
		"testChannel",
		&mockChannelConfig{},
		"testNet",
		&mockLocalMembership{id: &mockSigningIdentity{}},
		&mockConfigService{peerConf: &grpc.ConnectionConfig{Address: "peer1"}},
		&mockServices{peerClient: pc},
		ledger,
		&mockVault{},
		txMgr,
		nil,
		noop.NewTracerProvider(),
		nil,
		[]cb.HeaderType{cb.HeaderType_ENDORSER_TRANSACTION},
	)
	require.NoError(t, err)
	return svc
}

// newValidEnvelopeBytes builds a protobuf-encoded Envelope with the given header type and txID.
func newValidEnvelopeBytes(t *testing.T, headerType cb.HeaderType, txID string) []byte {
	t.Helper()
	channelHeaderBytes, err := proto.Marshal(&cb.ChannelHeader{
		Type: int32(headerType),
		TxId: txID,
	})
	require.NoError(t, err)
	payloadBytes, err := proto.Marshal(&cb.Payload{
		Header: &cb.Header{ChannelHeader: channelHeaderBytes},
	})
	require.NoError(t, err)
	envBytes, err := proto.Marshal(&cb.Envelope{Payload: payloadBytes})
	require.NoError(t, err)
	return envBytes
}
