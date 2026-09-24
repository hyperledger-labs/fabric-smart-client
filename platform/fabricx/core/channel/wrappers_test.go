/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channel

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	qsmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
)

// mockListenerManager records AddFinalityListener and RemoveFinalityListener calls safely across goroutines.
type mockListenerManager struct {
	mu          sync.Mutex
	addCalls    []addFinalityListenerCall
	removeCalls []removeFinalityListenerCall
	addErr      error
	removeErr   error
}

type addFinalityListenerCall struct {
	txID     string
	listener fdriver.FinalityListener
}

type removeFinalityListenerCall struct {
	txID     string
	listener fdriver.FinalityListener
}

func (m *mockListenerManager) AddFinalityListener(txID cdriver.TxID, listener fdriver.FinalityListener) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addCalls = append(m.addCalls, addFinalityListenerCall{txID: txID, listener: listener})
	return m.addErr
}

func (m *mockListenerManager) RemoveFinalityListener(txID cdriver.TxID, listener fdriver.FinalityListener) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeCalls = append(m.removeCalls, removeFinalityListenerCall{txID: txID, listener: listener})
	return m.removeErr
}

func (m *mockListenerManager) AddCallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.addCalls)
}

func (m *mockListenerManager) RemoveCallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.removeCalls)
}

func (m *mockListenerManager) GetAddCall(i int) addFinalityListenerCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addCalls[i]
}

func (m *mockListenerManager) GetRemoveCall(i int) removeFinalityListenerCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeCalls[i]
}

// --- fakeVault tests ---

func TestFakeVault_GetLastTxID(t *testing.T) {
	t.Parallel()
	v := &fakeVault{}
	txID, err := v.GetLastTxID(t.Context())
	require.NoError(t, err)
	assert.Empty(t, txID)
}

func TestFakeVault_GetLastBlock(t *testing.T) {
	t.Parallel()
	v := &fakeVault{}
	block, err := v.GetLastBlock(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(0), block)
}

// --- noopDeliveryService tests ---

func TestNoopDeliveryService_Start(t *testing.T) {
	t.Parallel()
	s := &noopDeliveryService{}
	require.NoError(t, s.Start(t.Context()))
}

// --- finalityListener tests ---

func TestFinalityListener_OnStatus(t *testing.T) {
	t.Parallel()

	var calledCtx context.Context
	var calledTxID string
	var calledStatus int
	var calledMsg string

	l := &finalityListener{
		onStatusFunc: func(ctx context.Context, txID string, status int, statusMessage string) {
			calledCtx = ctx
			calledTxID = txID
			calledStatus = status
			calledMsg = statusMessage
		},
	}

	ctx := t.Context()
	l.OnStatus(ctx, "tx1", fdriver.Valid, "ok")

	assert.Equal(t, ctx, calledCtx)
	assert.Equal(t, "tx1", calledTxID)
	assert.Equal(t, fdriver.Valid, calledStatus)
	assert.Equal(t, "ok", calledMsg)
}

// --- finalityServiceAdapter tests ---

func TestFinalityServiceAdapter_IsFinal_Valid(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	adapter := &finalityServiceAdapter{manager: mgr}

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- adapter.IsFinal(ctx, "tx-valid")
	}()

	require.Eventually(t, func() bool { return mgr.AddCallsCount() == 1 }, 100*time.Millisecond, time.Millisecond)
	listener := mgr.GetAddCall(0).listener

	listener.OnStatus(ctx, "tx-valid", fdriver.Valid, "committed")

	err := <-errCh
	require.NoError(t, err)
	assert.Equal(t, 1, mgr.RemoveCallsCount())
}

func TestFinalityServiceAdapter_IsFinal_Invalid(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	adapter := &finalityServiceAdapter{manager: mgr}

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- adapter.IsFinal(ctx, "tx-invalid")
	}()

	require.Eventually(t, func() bool { return mgr.AddCallsCount() == 1 }, 100*time.Millisecond, time.Millisecond)
	listener := mgr.GetAddCall(0).listener

	listener.OnStatus(ctx, "tx-invalid", fdriver.Invalid, "mvcc conflict")

	err := <-errCh
	require.ErrorContains(t, err, "is invalid")
	require.ErrorContains(t, err, "mvcc conflict")
}

func TestFinalityServiceAdapter_IsFinal_Unknown(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	adapter := &finalityServiceAdapter{manager: mgr}

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- adapter.IsFinal(ctx, "tx-unknown")
	}()

	require.Eventually(t, func() bool { return mgr.AddCallsCount() == 1 }, 100*time.Millisecond, time.Millisecond)
	listener := mgr.GetAddCall(0).listener

	listener.OnStatus(ctx, "tx-unknown", fdriver.Unknown, "timeout")

	err := <-errCh
	require.ErrorContains(t, err, "status is unknown")
}

func TestFinalityServiceAdapter_IsFinal_UnexpectedStatus(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	adapter := &finalityServiceAdapter{manager: mgr}

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- adapter.IsFinal(ctx, "tx-unexpected")
	}()

	require.Eventually(t, func() bool { return mgr.AddCallsCount() == 1 }, 100*time.Millisecond, time.Millisecond)
	listener := mgr.GetAddCall(0).listener

	listener.OnStatus(ctx, "tx-unexpected", 999, "weird")

	err := <-errCh
	require.ErrorContains(t, err, "unexpected status 999")
}

func TestFinalityServiceAdapter_IsFinal_AddListenerError(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{addErr: assert.AnError}
	adapter := &finalityServiceAdapter{manager: mgr}

	err := adapter.IsFinal(t.Context(), "tx-err")
	require.ErrorContains(t, err, "failed to add finality listener")
}

func TestFinalityServiceAdapter_IsFinal_ContextCancelled(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	adapter := &finalityServiceAdapter{manager: mgr}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := adapter.IsFinal(ctx, "tx-cancel")
	require.ErrorContains(t, err, "context cancelled")
}

func TestFinalityServiceAdapter_IsFinal_RemoveListenerError(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{removeErr: assert.AnError}
	adapter := &finalityServiceAdapter{manager: mgr}

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- adapter.IsFinal(ctx, "tx-rm-err")
	}()

	require.Eventually(t, func() bool { return mgr.AddCallsCount() == 1 }, 100*time.Millisecond, time.Millisecond)
	mgr.GetAddCall(0).listener.OnStatus(ctx, "tx-rm-err", fdriver.Valid, "ok")

	err := <-errCh
	require.NoError(t, err)
	assert.Equal(t, 1, mgr.RemoveCallsCount())
}

func TestFinalityServiceAdapter_IsFinal_DoubleNotification(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	adapter := &finalityServiceAdapter{manager: mgr}

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- adapter.IsFinal(ctx, "tx-double")
	}()

	require.Eventually(t, func() bool { return mgr.AddCallsCount() == 1 }, 100*time.Millisecond, time.Millisecond)
	listener := mgr.GetAddCall(0).listener

	// Call OnStatus twice to hit the default branch of select { case done <- ...: default: }
	listener.OnStatus(ctx, "tx-double", fdriver.Valid, "first")
	listener.OnStatus(ctx, "tx-double", fdriver.Valid, "second")

	err := <-errCh
	require.NoError(t, err)
}

// --- committerService tests ---

func TestCommitterService_Delegating_Methods(t *testing.T) {
	t.Parallel()
	cs := &committerService{}

	ctx := t.Context()

	require.NoError(t, cs.ReloadConfigTransactions())
	require.NoError(t, cs.Commit(ctx, &common.Block{}))
	require.NoError(t, cs.Start(ctx))
	require.NoError(t, cs.ProcessNamespace())
	require.NoError(t, cs.AddTransactionFilter(nil))
	require.NoError(t, cs.DiscardTx(ctx, "tx1", "reason"))
	require.NoError(t, cs.CommitTX(ctx, "tx1", 0, 0, nil))

	code, msg, err := cs.Status(ctx, "tx1")
	require.NoError(t, err)
	assert.Equal(t, fdriver.ValidationCode(0), code)
	assert.Empty(t, msg)
}

func TestCommitterService_IsFinal_NilFinalityService(t *testing.T) {
	t.Parallel()
	cs := &committerService{finalityService: nil}
	require.NoError(t, cs.IsFinal(t.Context(), "tx1"))
}

func TestCommitterService_IsFinal_WithFinalityService(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	fs := &finalityServiceAdapter{manager: mgr}
	cs := &committerService{finalityService: fs}

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- cs.IsFinal(ctx, "tx-cs")
	}()

	require.Eventually(t, func() bool { return mgr.AddCallsCount() == 1 }, 100*time.Millisecond, time.Millisecond)
	mgr.GetAddCall(0).listener.OnStatus(ctx, "tx-cs", fdriver.Valid, "ok")

	require.NoError(t, <-errCh)
}

func TestCommitterService_AddFinalityListener_NilService(t *testing.T) {
	t.Parallel()
	cs := &committerService{finalityService: nil}
	require.NoError(t, cs.AddFinalityListener("tx1", nil))
}

func TestCommitterService_AddFinalityListener_WithAdapter(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	fs := &finalityServiceAdapter{manager: mgr}
	cs := &committerService{finalityService: fs}

	require.NoError(t, cs.AddFinalityListener("tx-add", nil))
	require.Equal(t, 1, mgr.AddCallsCount())
	assert.Equal(t, "tx-add", mgr.GetAddCall(0).txID)
}

func TestCommitterService_RemoveFinalityListener_NilService(t *testing.T) {
	t.Parallel()
	cs := &committerService{finalityService: nil}
	require.NoError(t, cs.RemoveFinalityListener("tx1", nil))
}

func TestCommitterService_RemoveFinalityListener_WithAdapter(t *testing.T) {
	t.Parallel()
	mgr := &mockListenerManager{}
	fs := &finalityServiceAdapter{manager: mgr}
	cs := &committerService{finalityService: fs}

	require.NoError(t, cs.RemoveFinalityListener("tx-rm", nil))
	require.Equal(t, 1, mgr.RemoveCallsCount())
	assert.Equal(t, "tx-rm", mgr.GetRemoveCall(0).txID)
}

func TestCommitterService_AddFinalityListener_NonAdapterFinality(t *testing.T) {
	t.Parallel()
	cs := &committerService{finalityService: &plainFinality{}}
	require.NoError(t, cs.AddFinalityListener("tx1", nil))
}

func TestCommitterService_RemoveFinalityListener_NonAdapterFinality(t *testing.T) {
	t.Parallel()
	cs := &committerService{finalityService: &plainFinality{}}
	require.NoError(t, cs.RemoveFinalityListener("tx1", nil))
}

// plainFinality is a dummy Finality implementation for testing the non-adapter path.
type plainFinality struct{}

func (*plainFinality) IsFinal(context.Context, string) error { return nil }

// --- orderingServiceAdapter tests ---

func TestOrderingServiceAdapter_Configure_NotOrderingService(t *testing.T) {
	t.Parallel()
	adapter := &orderingServiceAdapter{os: &dummyOrdering{}}
	err := adapter.Configure("raft", nil)
	require.ErrorContains(t, err, "ordering service is not an *ordering.ChannelConfigMonitor")
}

func TestOrderingServiceAdapter_Configure_WithOrderingService(t *testing.T) {
	t.Parallel()

	mockCS := &mock.ConfigService{}
	mockCS.OrdererConnectionPoolSizeReturns(1)
	mockCS.NetworkNameReturns("testnet")
	mockCS.SetConfigOrderersReturns(nil)

	svc := ordering.NewService(nil, nil, mockCS, nil, dummyServices{})
	adapter := &orderingServiceAdapter{os: svc}

	// 1. "arma" consensus type translates to BFT
	err := adapter.Configure(armaConsensusType, nil)
	require.NoError(t, err)

	// 2. Passthrough for supported consensus types (e.g. Raft)
	err = adapter.Configure(ordering.Raft, nil)
	require.NoError(t, err)

	// 3. Unsupported consensus type fails
	err = adapter.Configure("unsupported_type", nil)
	require.ErrorContains(t, err, "failed to set consensus type")
}

// dummyOrdering implements fdriver.Ordering but is not *ordering.Service.
type dummyOrdering struct{}

func (*dummyOrdering) Broadcast(context.Context, any) error { return nil }
func (*dummyOrdering) SetConsensusType(string) error        { return nil }

type dummyServices struct{}

func (dummyServices) NewOrdererClient(_ grpc.ConnectionConfig) (ordering.Client, error) {
	return nil, nil
}

// --- startChannelConfigMonitor tests ---

type mockQueryServiceProvider struct {
	mu  sync.Mutex
	qs  queryservice.QueryService
	err error
}

func (m *mockQueryServiceProvider) Get(string, string) (queryservice.QueryService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.qs, m.err
}

type stubMembershipService struct {
	fdriver.MembershipService
}

func (*stubMembershipService) Update(*common.Envelope) error { return nil }
func (*stubMembershipService) OrdererConfig(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	return "", nil, nil
}

type stubFNS struct {
	fdriver.FabricNetworkService
	name            string
	configService   fdriver.ConfigService
	orderingService fdriver.Ordering
	localMembership fdriver.LocalMembership
}

func (s *stubFNS) Name() string                             { return s.name }
func (s *stubFNS) ConfigService() fdriver.ConfigService     { return s.configService }
func (s *stubFNS) OrderingService() fdriver.Ordering        { return s.orderingService }
func (s *stubFNS) LocalMembership() fdriver.LocalMembership { return s.localMembership }

func TestStartChannelConfigMonitor_QueryServiceError(t *testing.T) {
	t.Parallel()

	qsProvider := &mockQueryServiceProvider{err: assert.AnError}
	nw := &stubFNS{name: "testnet"}
	memService := &stubMembershipService{}

	monitor, err := startChannelConfigMonitor(nw, "mychannel", memService, qsProvider)
	require.ErrorContains(t, err, "failed to get query service for channel")
	assert.Nil(t, monitor)
}

func TestStartChannelConfigMonitor_NewConfigError(t *testing.T) {
	t.Parallel()

	mockCS := &mock.ConfigService{}
	mockCS.IsSetStub = func(k string) bool {
		return k == "configMonitor.pollInterval"
	}
	mockCS.GetDurationReturns(-1 * time.Second) // pollInterval <= 0 invalidates config

	qsProvider := &mockQueryServiceProvider{qs: &qsmock.QueryService{}}
	nw := &stubFNS{
		name:          "testnet",
		configService: mockCS,
	}
	memService := &stubMembershipService{}

	monitor, err := startChannelConfigMonitor(nw, "mychannel", memService, qsProvider)
	require.ErrorContains(t, err, "failed to create channel config monitor config")
	assert.Nil(t, monitor)
}

func TestStartChannelConfigMonitor_NewMonitorError(t *testing.T) {
	t.Parallel()

	mockCS := &mock.ConfigService{}
	qsProvider := &mockQueryServiceProvider{qs: &qsmock.QueryService{}}
	nw := &stubFNS{
		name:            "testnet",
		configService:   mockCS,
		orderingService: &dummyOrdering{},
	}
	memService := &stubMembershipService{}

	// Channel name empty causes NewChannelConfigMonitor to fail
	monitor, err := startChannelConfigMonitor(nw, "", memService, qsProvider)
	require.ErrorContains(t, err, "failed to create channel config monitor for channel []")
	assert.Nil(t, monitor)
}

func TestStartChannelConfigMonitor_SuccessAndStop(t *testing.T) {
	t.Parallel()

	mockCS := &mock.ConfigService{}
	mockCS.IsSetStub = func(k string) bool {
		return k == "configMonitor.maxRetries"
	}
	mockCS.GetIntReturns(0)

	mockQS := &qsmock.QueryService{}
	mockQS.GetConfigTransactionReturns(nil, assert.AnError)

	qsProvider := &mockQueryServiceProvider{qs: mockQS}
	nw := &stubFNS{
		name:            "testnet",
		configService:   mockCS,
		orderingService: &dummyOrdering{},
	}
	memService := &stubMembershipService{}

	monitor, err := startChannelConfigMonitor(nw, "mychannel", memService, qsProvider)
	require.NoError(t, err)
	require.NotNil(t, monitor)
	assert.True(t, monitor.IsRunning())

	// Stop cleanly to avoid leaking goroutines
	require.NoError(t, monitor.Stop())
	assert.False(t, monitor.IsRunning())
}

// --- ResolvingListenerManager compile-time check ---

var _ finality.ListenerManager = (*mockListenerManager)(nil)
