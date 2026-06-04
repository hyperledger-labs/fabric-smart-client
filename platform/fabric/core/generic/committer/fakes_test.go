/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

type testConfigService struct {
	fdriver.ConfigService
	networkName string
	peerAddress string
}

func (s *testConfigService) NetworkName() string {
	if s.networkName == "" {
		return "test-network"
	}
	return s.networkName
}

func (s *testConfigService) PickPeer(fdriver.PeerFunctionType) *grpc.ConnectionConfig {
	addr := s.peerAddress
	if addr == "" {
		addr = "peer0.test.net:7051"
	}
	return &grpc.ConnectionConfig{Address: addr}
}

type testChannelConfig struct {
	fdriver.ChannelConfig
	id                       string
	commitParallelism        int
	finalityRetries          int
	finalityUnknownTimeout   time.Duration
	waitForEventTimeout      time.Duration
	finalityEventQueueWorker int
}

func (c *testChannelConfig) ID() string {
	if c.id == "" {
		return "testchannel"
	}
	return c.id
}

func (c *testChannelConfig) CommitParallelism() int {
	if c.commitParallelism == 0 {
		return 1
	}
	return c.commitParallelism
}

func (c *testChannelConfig) CommitterFinalityNumRetries() int {
	if c.finalityRetries == 0 {
		return 1
	}
	return c.finalityRetries
}

func (c *testChannelConfig) CommitterFinalityUnknownTXTimeout() time.Duration {
	return c.finalityUnknownTimeout
}

func (c *testChannelConfig) CommitterWaitForEventTimeout() time.Duration {
	if c.waitForEventTimeout == 0 {
		return 10 * time.Millisecond
	}
	return c.waitForEventTimeout
}

func (c *testChannelConfig) FinalityEventQueueWorkers() int {
	if c.finalityEventQueueWorker == 0 {
		return 1
	}
	return c.finalityEventQueueWorker
}

type testPublisher struct {
	events     []events.Event
	publishedC chan events.Event
}

func (p *testPublisher) Publish(event events.Event) {
	p.events = append(p.events, event)
	if p.publishedC != nil {
		p.publishedC <- event
	}
}

type testVault struct {
	fdriver.Vault
	statusFn       func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error)
	setDiscardedFn func(context.Context, cdriver.TxID, string) error
	discardTxFn    func(context.Context, cdriver.TxID, string) error
	commitTxFn     func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error
	newRWSetFn     func(context.Context, cdriver.TxID) (fdriver.RWSet, error)
	rwsExistsFn    func(context.Context, cdriver.TxID) bool
	matchFn        func(context.Context, cdriver.TxID, []byte) error
}

func (v *testVault) Status(ctx context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
	if v.statusFn == nil {
		return fdriver.Unknown, "", nil
	}
	return v.statusFn(ctx, txID)
}

func (v *testVault) SetDiscarded(ctx context.Context, txID cdriver.TxID, message string) error {
	if v.setDiscardedFn == nil {
		return nil
	}
	return v.setDiscardedFn(ctx, txID, message)
}

func (v *testVault) DiscardTx(ctx context.Context, txID cdriver.TxID, message string) error {
	if v.discardTxFn == nil {
		return nil
	}
	return v.discardTxFn(ctx, txID, message)
}

func (v *testVault) CommitTX(ctx context.Context, txID cdriver.TxID, block cdriver.BlockNum, index cdriver.TxNum) error {
	if v.commitTxFn == nil {
		return nil
	}
	return v.commitTxFn(ctx, txID, block, index)
}

func (v *testVault) NewRWSet(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, error) {
	if v.newRWSetFn == nil {
		return &testRWSet{}, nil
	}
	return v.newRWSetFn(ctx, txID)
}

func (v *testVault) RWSExists(ctx context.Context, id cdriver.TxID) bool {
	if v.rwsExistsFn == nil {
		return false
	}
	return v.rwsExistsFn(ctx, id)
}

func (v *testVault) Match(ctx context.Context, id cdriver.TxID, results []byte) error {
	if v.matchFn == nil {
		return nil
	}
	return v.matchFn(ctx, id, results)
}

type testEnvelopeService struct {
	fdriver.EnvelopeService
	existsFn       func(context.Context, string) bool
	storeEnvelope  func(context.Context, string, any) error
	loadEnvelopeFn func(context.Context, string) ([]byte, error)
}

func (s *testEnvelopeService) Exists(ctx context.Context, txid string) bool {
	if s.existsFn == nil {
		return false
	}
	return s.existsFn(ctx, txid)
}

func (s *testEnvelopeService) StoreEnvelope(ctx context.Context, txid string, env any) error {
	if s.storeEnvelope == nil {
		return nil
	}
	return s.storeEnvelope(ctx, txid, env)
}

func (s *testEnvelopeService) LoadEnvelope(ctx context.Context, txid string) ([]byte, error) {
	if s.loadEnvelopeFn == nil {
		return nil, nil
	}
	return s.loadEnvelopeFn(ctx, txid)
}

type testRWSet struct {
	fdriver.RWSet
	namespaces []cdriver.Namespace
	readKeys   map[cdriver.Namespace][]string
	doneCount  int
	setStateFn func(cdriver.Namespace, cdriver.PKey, cdriver.RawValue) error
}

func (r *testRWSet) Namespaces() []cdriver.Namespace {
	return r.namespaces
}

func (r *testRWSet) NumReads(ns cdriver.Namespace) int {
	return len(r.readKeys[ns])
}

func (r *testRWSet) GetReadKeyAt(ns cdriver.Namespace, i int) (cdriver.PKey, error) {
	keys := r.readKeys[ns]
	if i < 0 || i >= len(keys) {
		return "", stderrors.New("read index out of range")
	}
	return keys[i], nil
}

func (r *testRWSet) Done() {
	r.doneCount++
}

func (r *testRWSet) SetState(namespace cdriver.Namespace, key cdriver.PKey, value cdriver.RawValue) error {
	if r.setStateFn == nil {
		return nil
	}
	return r.setStateFn(namespace, key, value)
}

type testRWSetLoader struct {
	fdriver.RWSetLoader
	getFromEnvelopeFn func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error)
	getFromETxFn      func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error)
	inspectFromEnvFn  func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error)
}

func (l *testRWSetLoader) GetRWSetFromEvn(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	if l.getFromEnvelopeFn == nil {
		return &testRWSet{}, nil, nil
	}
	return l.getFromEnvelopeFn(ctx, txID)
}

func (l *testRWSetLoader) GetRWSetFromETx(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	if l.getFromETxFn == nil {
		return &testRWSet{}, nil, nil
	}
	return l.getFromETxFn(ctx, txID)
}

func (l *testRWSetLoader) GetInspectingRWSetFromEvn(ctx context.Context, id cdriver.TxID, envelopeRaw []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	if l.inspectFromEnvFn == nil {
		return &testRWSet{}, nil, nil
	}
	return l.inspectFromEnvFn(ctx, id, envelopeRaw)
}

type testProcessorManager struct {
	fdriver.ProcessorManager
	processByIDFn func(context.Context, string, cdriver.TxID) error
}

func (m *testProcessorManager) ProcessByID(ctx context.Context, channel string, txid cdriver.TxID) error {
	if m.processByIDFn == nil {
		return nil
	}
	return m.processByIDFn(ctx, channel, txid)
}

type testProcessedTransaction struct {
	fdriver.ProcessedTransaction
	valid          bool
	validationCode int32
	envelope       []byte
	results        []byte
}

func (pt *testProcessedTransaction) IsValid() bool {
	return pt.valid
}

func (pt *testProcessedTransaction) ValidationCode() int32 {
	return pt.validationCode
}

func (pt *testProcessedTransaction) Envelope() []byte {
	return pt.envelope
}

func (pt *testProcessedTransaction) Results() []byte {
	return pt.results
}

type testLedger struct {
	fdriver.Ledger
	getByIDFn func(string) (fdriver.ProcessedTransaction, error)
}

func (l *testLedger) GetTransactionByID(txID string) (fdriver.ProcessedTransaction, error) {
	if l.getByIDFn == nil {
		return nil, stderrors.New("missing test GetTransactionByID implementation")
	}
	return l.getByIDFn(txID)
}

type testFabricFinality struct {
	err error
}

func (f *testFabricFinality) IsFinal(string, string) error {
	return f.err
}

type testTransactionManager struct {
	fdriver.TransactionManager
	newProcessedFromPayloadFn func([]byte) (fdriver.ProcessedTransaction, int32, error)
}

func (m *testTransactionManager) NewProcessedTransactionFromEnvelopePayload(envelopePayload []byte) (fdriver.ProcessedTransaction, int32, error) {
	if m.newProcessedFromPayloadFn == nil {
		return nil, -1, nil
	}
	return m.newProcessedFromPayloadFn(envelopePayload)
}

type testCounter struct{}

func (c *testCounter) With(...string) metrics.Counter {
	return c
}

func (c *testCounter) Add(float64) {}

type testGauge struct{}

func (g *testGauge) With(...string) metrics.Gauge {
	return g
}

func (g *testGauge) Add(float64) {}

func (g *testGauge) Set(float64) {}

type testHistogram struct{}

func (h *testHistogram) With(...string) metrics.Histogram {
	return h
}

func (h *testHistogram) Observe(float64) {}

type testMetricsProvider struct{}

func (p *testMetricsProvider) NewCounter(metrics.CounterOpts) metrics.Counter {
	return &testCounter{}
}

func (p *testMetricsProvider) NewGauge(metrics.GaugeOpts) metrics.Gauge {
	return &testGauge{}
}

func (p *testMetricsProvider) NewHistogram(metrics.HistogramOpts) metrics.Histogram {
	return &testHistogram{}
}

type testFilter struct {
	accept bool
	err    error
}

func (f *testFilter) Accept(cdriver.TxID, []byte) (bool, error) {
	return f.accept, f.err
}

type testFinalityListener struct{}

func (l *testFinalityListener) OnStatus(context.Context, cdriver.TxID, fdriver.ValidationCode, string) {
}

type testListenerManager struct {
	addedTx   cdriver.TxID
	removedTx cdriver.TxID
}

func (m *testListenerManager) AddListener(txID cdriver.TxID, _ cdriver.FinalityListener[fdriver.ValidationCode]) error {
	m.addedTx = txID
	return nil
}

func (m *testListenerManager) RemoveListener(txID cdriver.TxID, _ cdriver.FinalityListener[fdriver.ValidationCode]) {
	m.removedTx = txID
}

func (m *testListenerManager) InvokeListeners(cdriver.FinalityEvent[fdriver.ValidationCode]) {}

func (m *testListenerManager) TxIDs() []cdriver.TxID { return nil }

type testQueryExecutor struct {
	cdriver.QueryExecutor
	doneErr     error
	getStateFn  func(context.Context, cdriver.Namespace, cdriver.PKey) (*cdriver.VaultRead, error)
	getStateMD  func(context.Context, cdriver.Namespace, cdriver.PKey) (cdriver.Metadata, cdriver.RawVersion, error)
	getStateRng func(context.Context, cdriver.Namespace, cdriver.PKey, cdriver.PKey) (cdriver.VersionedResultsIterator, error)
}

func (q *testQueryExecutor) GetState(ctx context.Context, namespace cdriver.Namespace, key cdriver.PKey) (*cdriver.VaultRead, error) {
	if q.getStateFn == nil {
		return nil, nil
	}
	return q.getStateFn(ctx, namespace, key)
}

func (q *testQueryExecutor) GetStateMetadata(ctx context.Context, namespace cdriver.Namespace, key cdriver.PKey) (cdriver.Metadata, cdriver.RawVersion, error) {
	if q.getStateMD == nil {
		return nil, nil, nil
	}
	return q.getStateMD(ctx, namespace, key)
}

func (q *testQueryExecutor) GetStateRange(ctx context.Context, namespace cdriver.Namespace, startKey, endKey cdriver.PKey) (cdriver.VersionedResultsIterator, error) {
	if q.getStateRng == nil {
		return nil, nil
	}
	return q.getStateRng(ctx, namespace, startKey, endKey)
}

func (q *testQueryExecutor) Done() error {
	return q.doneErr
}

type testMembershipService struct {
	fdriver.MembershipService
	ordererConfigFn func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error)
	updateFn        func(*common.Envelope) error
}

func (m *testMembershipService) OrdererConfig(cs fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	if m.ordererConfigFn == nil {
		return "", nil, nil
	}
	return m.ordererConfigFn(cs)
}

func (m *testMembershipService) Update(env *common.Envelope) error {
	if m.updateFn == nil {
		return nil
	}
	return m.updateFn(env)
}

type testOrderingService struct {
	consensusType string
	endpoints     []*grpc.ConnectionConfig
	err           error
	configureFn   func(string, []*grpc.ConnectionConfig) error
}

func (s *testOrderingService) Configure(consensusType string, orderers []*grpc.ConnectionConfig) error {
	s.consensusType = consensusType
	s.endpoints = orderers
	if s.configureFn != nil {
		return s.configureFn(consensusType, orderers)
	}
	return s.err
}

type testVaultWithQueryErr struct {
	testVault
	err error
}

func (v *testVaultWithQueryErr) NewQueryExecutor(context.Context) (cdriver.QueryExecutor, error) {
	return nil, v.err
}

type testVaultWithQuery struct {
	testVault
	qe cdriver.QueryExecutor
}

func (v *testVaultWithQuery) NewQueryExecutor(context.Context) (cdriver.QueryExecutor, error) {
	return v.qe, nil
}
