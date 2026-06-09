/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

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

type ConfigService struct {
	fdriver.ConfigService
	NetworkNameValue string
	PeerAddress      string
}

func (s *ConfigService) NetworkName() string {
	if s.NetworkNameValue == "" {
		return "test-network"
	}
	return s.NetworkNameValue
}

func (s *ConfigService) PickPeer(fdriver.PeerFunctionType) *grpc.ConnectionConfig {
	addr := s.PeerAddress
	if addr == "" {
		addr = "peer0.test.net:7051"
	}
	return &grpc.ConnectionConfig{Address: addr}
}

type ChannelConfig struct {
	fdriver.ChannelConfig
	IDValue                  string
	CommitParallelismValue   int
	FinalityRetries          int
	FinalityUnknownTimeout   time.Duration
	WaitForEventTimeout      time.Duration
	FinalityEventQueueWorker int
}

func (c *ChannelConfig) ID() string {
	if c.IDValue == "" {
		return "testchannel"
	}
	return c.IDValue
}

func (c *ChannelConfig) CommitParallelism() int {
	if c.CommitParallelismValue == 0 {
		return 1
	}
	return c.CommitParallelismValue
}

func (c *ChannelConfig) CommitterFinalityNumRetries() int {
	if c.FinalityRetries == 0 {
		return 1
	}
	return c.FinalityRetries
}

func (c *ChannelConfig) CommitterFinalityUnknownTXTimeout() time.Duration {
	return c.FinalityUnknownTimeout
}

func (c *ChannelConfig) CommitterWaitForEventTimeout() time.Duration {
	if c.WaitForEventTimeout == 0 {
		return 10 * time.Millisecond
	}
	return c.WaitForEventTimeout
}

func (c *ChannelConfig) FinalityEventQueueWorkers() int {
	if c.FinalityEventQueueWorker == 0 {
		return 1
	}
	return c.FinalityEventQueueWorker
}

type Publisher struct {
	Events     []events.Event
	PublishedC chan events.Event
}

func (p *Publisher) Publish(event events.Event) {
	p.Events = append(p.Events, event)
	if p.PublishedC != nil {
		p.PublishedC <- event
	}
}

type Vault struct {
	fdriver.Vault
	StatusFn       func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error)
	SetDiscardedFn func(context.Context, cdriver.TxID, string) error
	DiscardTxFn    func(context.Context, cdriver.TxID, string) error
	CommitTxFn     func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error
	NewRWSetFn     func(context.Context, cdriver.TxID) (fdriver.RWSet, error)
	RWSExistsFn    func(context.Context, cdriver.TxID) bool
	MatchFn        func(context.Context, cdriver.TxID, []byte) error
}

func (v *Vault) Status(ctx context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
	if v.StatusFn == nil {
		return fdriver.Unknown, "", nil
	}
	return v.StatusFn(ctx, txID)
}

func (v *Vault) SetDiscarded(ctx context.Context, txID cdriver.TxID, message string) error {
	if v.SetDiscardedFn == nil {
		return nil
	}
	return v.SetDiscardedFn(ctx, txID, message)
}

func (v *Vault) DiscardTx(ctx context.Context, txID cdriver.TxID, message string) error {
	if v.DiscardTxFn == nil {
		return nil
	}
	return v.DiscardTxFn(ctx, txID, message)
}

func (v *Vault) CommitTX(ctx context.Context, txID cdriver.TxID, block cdriver.BlockNum, index cdriver.TxNum) error {
	if v.CommitTxFn == nil {
		return nil
	}
	return v.CommitTxFn(ctx, txID, block, index)
}

func (v *Vault) NewRWSet(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, error) {
	if v.NewRWSetFn == nil {
		return &RWSet{}, nil
	}
	return v.NewRWSetFn(ctx, txID)
}

func (v *Vault) RWSExists(ctx context.Context, id cdriver.TxID) bool {
	if v.RWSExistsFn == nil {
		return false
	}
	return v.RWSExistsFn(ctx, id)
}

func (v *Vault) Match(ctx context.Context, id cdriver.TxID, results []byte) error {
	if v.MatchFn == nil {
		return nil
	}
	return v.MatchFn(ctx, id, results)
}

type EnvelopeService struct {
	fdriver.EnvelopeService
	ExistsFn        func(context.Context, string) bool
	StoreEnvelopeFn func(context.Context, string, any) error
	LoadEnvelopeFn  func(context.Context, string) ([]byte, error)
}

func (s *EnvelopeService) Exists(ctx context.Context, txid string) bool {
	if s.ExistsFn == nil {
		return false
	}
	return s.ExistsFn(ctx, txid)
}

func (s *EnvelopeService) StoreEnvelope(ctx context.Context, txid string, env any) error {
	if s.StoreEnvelopeFn == nil {
		return nil
	}
	return s.StoreEnvelopeFn(ctx, txid, env)
}

func (s *EnvelopeService) LoadEnvelope(ctx context.Context, txid string) ([]byte, error) {
	if s.LoadEnvelopeFn == nil {
		return nil, nil
	}
	return s.LoadEnvelopeFn(ctx, txid)
}

type RWSet struct {
	fdriver.RWSet
	NamespacesList []cdriver.Namespace
	ReadKeys       map[cdriver.Namespace][]string
	DoneCount      int
	SetStateFn     func(cdriver.Namespace, cdriver.PKey, cdriver.RawValue) error
}

func (r *RWSet) Namespaces() []cdriver.Namespace {
	return r.NamespacesList
}

func (r *RWSet) NumReads(ns cdriver.Namespace) int {
	return len(r.ReadKeys[ns])
}

func (r *RWSet) GetReadKeyAt(ns cdriver.Namespace, i int) (cdriver.PKey, error) {
	keys := r.ReadKeys[ns]
	if i < 0 || i >= len(keys) {
		return "", stderrors.New("read index out of range")
	}
	return keys[i], nil
}

func (r *RWSet) Done() {
	r.DoneCount++
}

func (r *RWSet) SetState(namespace cdriver.Namespace, key cdriver.PKey, value cdriver.RawValue) error {
	if r.SetStateFn == nil {
		return nil
	}
	return r.SetStateFn(namespace, key, value)
}

type RWSetLoader struct {
	fdriver.RWSetLoader
	GetFromEnvelopeFn func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error)
	GetFromETxFn      func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error)
	InspectFromEnvFn  func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error)
}

func (l *RWSetLoader) GetRWSetFromEvn(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	if l.GetFromEnvelopeFn == nil {
		return &RWSet{}, nil, nil
	}
	return l.GetFromEnvelopeFn(ctx, txID)
}

func (l *RWSetLoader) GetRWSetFromETx(ctx context.Context, txID cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	if l.GetFromETxFn == nil {
		return &RWSet{}, nil, nil
	}
	return l.GetFromETxFn(ctx, txID)
}

func (l *RWSetLoader) GetInspectingRWSetFromEvn(ctx context.Context, id cdriver.TxID, envelopeRaw []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
	if l.InspectFromEnvFn == nil {
		return &RWSet{}, nil, nil
	}
	return l.InspectFromEnvFn(ctx, id, envelopeRaw)
}

type ProcessorManager struct {
	fdriver.ProcessorManager
	ProcessByIDFn func(context.Context, string, cdriver.TxID) error
}

func (m *ProcessorManager) ProcessByID(ctx context.Context, channel string, txid cdriver.TxID) error {
	if m.ProcessByIDFn == nil {
		return nil
	}
	return m.ProcessByIDFn(ctx, channel, txid)
}

type ProcessedTransaction struct {
	fdriver.ProcessedTransaction
	Valid               bool
	ValidationCodeValue int32
	EnvelopeBytes       []byte
	ResultsBytes        []byte
}

func (pt *ProcessedTransaction) IsValid() bool {
	return pt.Valid
}

func (pt *ProcessedTransaction) ValidationCode() int32 {
	return pt.ValidationCodeValue
}

func (pt *ProcessedTransaction) Envelope() []byte {
	return pt.EnvelopeBytes
}

func (pt *ProcessedTransaction) Results() []byte {
	return pt.ResultsBytes
}

type Ledger struct {
	fdriver.Ledger
	GetByIDFn func(string) (fdriver.ProcessedTransaction, error)
}

func (l *Ledger) GetTransactionByID(txID string) (fdriver.ProcessedTransaction, error) {
	if l.GetByIDFn == nil {
		return nil, stderrors.New("missing test GetTransactionByID implementation")
	}
	return l.GetByIDFn(txID)
}

type FabricFinality struct {
	Err error
}

func (f *FabricFinality) IsFinal(string, string) error {
	return f.Err
}

type TransactionManager struct {
	fdriver.TransactionManager
	NewProcessedFromPayloadFn func([]byte) (fdriver.ProcessedTransaction, int32, error)
}

func (m *TransactionManager) NewProcessedTransactionFromEnvelopePayload(envelopePayload []byte) (fdriver.ProcessedTransaction, int32, error) {
	if m.NewProcessedFromPayloadFn == nil {
		return nil, -1, nil
	}
	return m.NewProcessedFromPayloadFn(envelopePayload)
}

type Counter struct{}

func (c *Counter) With(...string) metrics.Counter {
	return c
}

func (c *Counter) Add(float64) {}

type Gauge struct{}

func (g *Gauge) With(...string) metrics.Gauge {
	return g
}

func (g *Gauge) Add(float64) {}

func (g *Gauge) Set(float64) {}

type Histogram struct{}

func (h *Histogram) With(...string) metrics.Histogram {
	return h
}

func (h *Histogram) Observe(float64) {}

type MetricsProvider struct{}

func (p *MetricsProvider) NewCounter(metrics.CounterOpts) metrics.Counter {
	return &Counter{}
}

func (p *MetricsProvider) NewGauge(metrics.GaugeOpts) metrics.Gauge {
	return &Gauge{}
}

func (p *MetricsProvider) NewHistogram(metrics.HistogramOpts) metrics.Histogram {
	return &Histogram{}
}

type Filter struct {
	AcceptValue bool
	Err         error
}

func (f *Filter) Accept(cdriver.TxID, []byte) (bool, error) {
	return f.AcceptValue, f.Err
}

type FinalityListener struct{}

func (l *FinalityListener) OnStatus(context.Context, cdriver.TxID, fdriver.ValidationCode, string) {
}

type ListenerManager struct {
	AddedTx   cdriver.TxID
	RemovedTx cdriver.TxID
}

func (m *ListenerManager) AddListener(txID cdriver.TxID, _ cdriver.FinalityListener[fdriver.ValidationCode]) error {
	m.AddedTx = txID
	return nil
}

func (m *ListenerManager) RemoveListener(txID cdriver.TxID, _ cdriver.FinalityListener[fdriver.ValidationCode]) {
	m.RemovedTx = txID
}

func (m *ListenerManager) InvokeListeners(cdriver.FinalityEvent[fdriver.ValidationCode]) {}

func (m *ListenerManager) TxIDs() []cdriver.TxID { return nil }

type QueryExecutor struct {
	cdriver.QueryExecutor
	DoneErr         error
	GetStateFn      func(context.Context, cdriver.Namespace, cdriver.PKey) (*cdriver.VaultRead, error)
	GetStateMDFn    func(context.Context, cdriver.Namespace, cdriver.PKey) (cdriver.Metadata, cdriver.RawVersion, error)
	GetStateRangeFn func(context.Context, cdriver.Namespace, cdriver.PKey, cdriver.PKey) (cdriver.VersionedResultsIterator, error)
}

func (q *QueryExecutor) GetState(ctx context.Context, namespace cdriver.Namespace, key cdriver.PKey) (*cdriver.VaultRead, error) {
	if q.GetStateFn == nil {
		return nil, nil
	}
	return q.GetStateFn(ctx, namespace, key)
}

func (q *QueryExecutor) GetStateMetadata(ctx context.Context, namespace cdriver.Namespace, key cdriver.PKey) (cdriver.Metadata, cdriver.RawVersion, error) {
	if q.GetStateMDFn == nil {
		return nil, nil, nil
	}
	return q.GetStateMDFn(ctx, namespace, key)
}

func (q *QueryExecutor) GetStateRange(ctx context.Context, namespace cdriver.Namespace, startKey, endKey cdriver.PKey) (cdriver.VersionedResultsIterator, error) {
	if q.GetStateRangeFn == nil {
		return nil, nil
	}
	return q.GetStateRangeFn(ctx, namespace, startKey, endKey)
}

func (q *QueryExecutor) Done() error {
	return q.DoneErr
}

type MembershipService struct {
	fdriver.MembershipService
	OrdererConfigFn func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error)
	UpdateFn        func(*common.Envelope) error
}

func (m *MembershipService) OrdererConfig(cs fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	if m.OrdererConfigFn == nil {
		return "", nil, nil
	}
	return m.OrdererConfigFn(cs)
}

func (m *MembershipService) Update(env *common.Envelope) error {
	if m.UpdateFn == nil {
		return nil
	}
	return m.UpdateFn(env)
}

type OrderingService struct {
	ConsensusType string
	Endpoints     []*grpc.ConnectionConfig
	Err           error
	ConfigureFn   func(string, []*grpc.ConnectionConfig) error
}

func (s *OrderingService) Configure(consensusType string, orderers []*grpc.ConnectionConfig) error {
	s.ConsensusType = consensusType
	s.Endpoints = orderers
	if s.ConfigureFn != nil {
		return s.ConfigureFn(consensusType, orderers)
	}
	return s.Err
}

type VaultWithQueryErr struct {
	Vault
	Err error
}

func (v *VaultWithQueryErr) NewQueryExecutor(context.Context) (cdriver.QueryExecutor, error) {
	return nil, v.Err
}

type VaultWithQuery struct {
	Vault
	QE cdriver.QueryExecutor
}

func (v *VaultWithQuery) NewQueryExecutor(context.Context) (cdriver.QueryExecutor, error) {
	return v.QE, nil
}
