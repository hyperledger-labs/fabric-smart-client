/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

const (
	testBlockNumber        driver.BlockNum = 7
	testListenerBufferSize                 = 4
	testListenerTimeout                    = 2 * time.Second
)

func TestAddEventListenerInvokesImmediatelyForCachedEvent(t *testing.T) {
	t.Parallel()

	manager := newTestListenerManager(t)
	manager.events.Put("tx1", testEvent{id: "tx1"})

	listener := newTestListener("listener-1")
	err := manager.AddEventListener("tx1", listener)
	require.NoError(t, err)

	require.Equal(t, "tx1", waitForEvent(t, listener.calls).ID())
	_, ok := manager.listeners.Get("tx1")
	require.False(t, ok)
}

func TestAddPermanentEventListenerInvokesImmediatelyForCachedEvent(t *testing.T) {
	t.Parallel()

	manager := newTestListenerManager(t)
	manager.events.Put("tx1", testEvent{id: "tx1"})

	listener := newTestListener("listener-1")
	err := manager.AddPermanentEventListener("tx1", listener)
	require.NoError(t, err)

	require.Equal(t, "tx1", waitForEvent(t, listener.calls).ID())
	require.Len(t, manager.permanentListeners["tx1"], 1)
}

// TestRemoveEventListenerRemovesMatchingListener verifies that removing one of
// two registered listeners leaves exactly one listener registered, and that
// removing the remaining listener succeeds while a subsequent removal of the
// same listener returns an error. The assertions are expressed entirely through
// the public API so the test is not coupled to the internal cache layout.
func TestRemoveEventListenerRemovesMatchingListener(t *testing.T) {
	t.Parallel()

	manager := newTestListenerManager(t)
	listenerOne := newTestListener("listener-1")
	listenerTwo := newTestListener("listener-2")

	require.NoError(t, manager.AddEventListener("tx1", listenerOne))
	require.NoError(t, manager.AddEventListener("tx1", listenerTwo))

	// Remove listenerOne — the slot still holds listenerTwo, so removal succeeds.
	require.NoError(t, manager.RemoveEventListener("tx1", listenerOne))

	// listenerTwo is still registered — removing it should succeed.
	require.NoError(t, manager.RemoveEventListener("tx1", listenerTwo))

	// The slot is now empty — a second attempt to remove listenerTwo must fail.
	err := manager.RemoveEventListener("tx1", listenerTwo)
	require.ErrorContains(t, err, "could not find listener [")
	require.ErrorContains(t, err, "listener-2")
	require.ErrorContains(t, err, "in eventID [tx1]")
}

func TestRemoveEventListenerReturnsErrorWhenMissing(t *testing.T) {
	t.Parallel()

	manager := newTestListenerManager(t)

	err := manager.RemoveEventListener("tx1", newTestListener("listener-1"))
	require.ErrorContains(t, err, "could not find listener [")
	require.ErrorContains(t, err, "listener-1")
	require.ErrorContains(t, err, "in eventID [tx1]")
}

// TestRemoveEventListenerAfterListenerAlreadyInvoked verifies that calling
// RemoveEventListener for a listener that was already invoked and cleaned up by
// onBlock returns an error. This mirrors the realistic scenario where a caller
// tries to cancel a subscription after the corresponding transaction has already
// reached finality.
func TestRemoveEventListenerAfterListenerAlreadyInvoked(t *testing.T) {
	t.Parallel()

	manager := newTestListenerManager(t)
	listener := newTestListener("listener-1")

	require.NoError(t, manager.AddEventListener("tx1", listener))

	// Simulate onBlock processing the transaction and invoking the listener.
	manager.mapper = &parallelBlockMapper[testEvent]{
		logger: manager.logger,
		cap:    1,
		mapper: staticMapper{
			eventsByTx: []map[driver.Namespace]testEvent{
				{"ns1": {id: "tx1"}},
			},
		},
	}
	err := manager.onBlock(context.Background(), &common.Block{
		Header:   &common.BlockHeader{Number: uint64(testBlockNumber)},
		Data:     &common.BlockData{Data: [][]byte{[]byte("tx-1")}},
		Metadata: &common.BlockMetadata{},
	})
	require.NoError(t, err)

	// Wait for the listener to be invoked so we know onBlock has completed.
	require.Equal(t, "tx1", waitForEvent(t, listener.calls).ID())

	// At this point, onBlock has already removed the listener from the internal
	// map. A subsequent RemoveEventListener call must return an error because
	// there is nothing left to remove.
	err = manager.RemoveEventListener("tx1", listener)
	require.ErrorContains(t, err, "could not find listener [")
	require.ErrorContains(t, err, "listener-1")
	require.ErrorContains(t, err, "in eventID [tx1]")
}

func TestOnBlockInvokesTransientAndPermanentListenersAndCachesEvent(t *testing.T) {
	t.Parallel()

	manager := newTestListenerManager(t)
	transient := newTestListener("transient")
	permanent := newTestListener("permanent")

	require.NoError(t, manager.AddEventListener("tx1", transient))
	require.NoError(t, manager.AddPermanentEventListener("tx1", permanent))

	manager.mapper = &parallelBlockMapper[testEvent]{
		logger: manager.logger,
		cap:    1,
		mapper: staticMapper{
			eventsByTx: []map[driver.Namespace]testEvent{
				{
					"ns1": {id: "tx1"},
				},
			},
		},
	}

	err := manager.onBlock(context.Background(), &common.Block{
		Header:   &common.BlockHeader{Number: uint64(testBlockNumber)},
		Data:     &common.BlockData{Data: [][]byte{[]byte("tx-1")}},
		Metadata: &common.BlockMetadata{},
	})
	require.NoError(t, err)

	require.Equal(t, "tx1", waitForEvent(t, transient.calls).ID())
	require.Equal(t, "tx1", waitForEvent(t, permanent.calls).ID())
	require.Equal(t, testBlockNumber, manager.lastBlockNum.Load())

	event, ok := manager.events.Get("tx1")
	require.True(t, ok)
	require.Equal(t, testEvent{id: "tx1"}, event)

	_, ok = manager.listeners.Get("tx1")
	require.False(t, ok)
	require.Len(t, manager.permanentListeners["tx1"], 1)
}

func TestNewListenerManagerStartsDeliveryAndRegistersListenersCache(t *testing.T) {
	t.Parallel()

	delivery := &recordingDelivery{called: make(chan fabric.BlockCallback, 1)}
	manager, err := NewListenerManager[testEvent](
		testLogger(t),
		DeliveryListenerManagerConfig{
			ListenerTimeout: testListenerTimeout,
		},
		delivery,
		&staticQueryService{},
		nil,
		staticMapper{},
	)
	require.NoError(t, err)

	callback := waitForCallback(t, delivery.called)
	require.NotNil(t, callback)
	require.NotNil(t, manager.listeners)
	require.NotNil(t, manager.events)
}

func TestNewBlockCallbackReturnsErrorWhenBlockProcessingFails(t *testing.T) {
	t.Parallel()

	manager := newTestListenerManager(t)
	manager.ignoreBlockErrors = false
	manager.mapper = &parallelBlockMapper[testEvent]{
		logger: manager.logger,
		cap:    1,
		mapper: staticMapper{err: errors.New("boom")},
	}

	stop, err := manager.newBlockCallback()(context.Background(), &common.Block{
		Header:   &common.BlockHeader{Number: uint64(testBlockNumber)},
		Data:     &common.BlockData{Data: [][]byte{[]byte("tx-1")}},
		Metadata: &common.BlockMetadata{},
	})
	require.True(t, stop)
	require.ErrorContains(t, err, "failed to process block")
}

func TestNewBlockCallbackWithParallelProcessingDoesNotBubbleErrors(t *testing.T) {
	t.Parallel()

	manager := newTestListenerManager(t)
	manager.blockProcessingParallelism = 2
	manager.mapper = &parallelBlockMapper[testEvent]{
		logger: manager.logger,
		cap:    1,
		mapper: staticMapper{err: errors.New("boom")},
	}

	stop, err := manager.newBlockCallback()(context.Background(), &common.Block{
		Header:   &common.BlockHeader{Number: uint64(testBlockNumber)},
		Data:     &common.BlockData{Data: [][]byte{[]byte("tx-1")}},
		Metadata: &common.BlockMetadata{},
	})
	require.False(t, stop)
	require.NoError(t, err)
}

func TestFetchTxsNotifiesEvictedListeners(t *testing.T) {
	t.Parallel()

	listener := newTestListener("listener-1")
	service := &staticQueryService{
		events: []testEvent{{id: "tx1"}},
	}

	fetchTxs(
		context.Background(),
		testLogger(t),
		testBlockNumber,
		map[EventID][]ListenerEntry[testEvent]{
			"tx1": {listener},
		},
		service,
		true,
	)

	require.Equal(t, "tx1", waitForEvent(t, listener.calls).ID())
	require.Equal(t, testBlockNumber, service.startingBlock)
}

func TestParallelBlockMapperMapReturnsError(t *testing.T) {
	t.Parallel()

	mapper := &parallelBlockMapper[testEvent]{
		logger: testLogger(t),
		cap:    1,
		mapper: staticMapper{err: errors.New("boom")},
	}

	_, err := mapper.Map(context.Background(), &common.Block{
		Header:   &common.BlockHeader{Number: uint64(testBlockNumber)},
		Data:     &common.BlockData{Data: [][]byte{[]byte("tx-1")}},
		Metadata: &common.BlockMetadata{},
	})
	require.EqualError(t, err, "boom")
}

// testEvent is a minimal EventInfo implementation used in tests.
type testEvent struct {
	id string
}

// ID implements EventInfo and returns the unique identifier for the event.
func (e testEvent) ID() EventID {
	return e.id
}

// testListener is a ListenerEntry stub that records every OnStatus invocation
// in a buffered channel so tests can assert on the received events.
type testListener struct {
	id    string
	calls chan testEvent
}

// newTestListener creates a testListener with the given id and a pre-allocated
// calls channel sized to testListenerBufferSize.
func newTestListener(id string) *testListener {
	return &testListener{
		id:    id,
		calls: make(chan testEvent, testListenerBufferSize),
	}
}

// Namespace implements ListenerEntry and returns an empty namespace, meaning
// the listener applies to all namespaces.
func (l *testListener) Namespace() driver.Namespace {
	return ""
}

// OnStatus implements ListenerEntry and forwards the received event to the
// calls channel so the test can observe it.
func (l *testListener) OnStatus(_ context.Context, info testEvent) {
	l.calls <- info
}

// Equals implements ListenerEntry and reports whether other is a *testListener
// with the same id.
func (l *testListener) Equals(other ListenerEntry[testEvent]) bool {
	otherListener, ok := other.(*testListener)
	return ok && otherListener.id == l.id
}

// staticMapper is an EventInfoMapper stub that returns a fixed set of events
// per transaction index. If err is non-nil, MapTxData returns that error
// instead.
type staticMapper struct {
	eventsByTx []map[driver.Namespace]testEvent
	err        error
}

// MapTxData returns the pre-configured namespace→event map for the given txNum,
// or err if it is set.
func (m staticMapper) MapTxData(_ context.Context, _ []byte, _ *common.BlockMetadata, _ driver.BlockNum, txNum driver.TxNum) (map[driver.Namespace]testEvent, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.eventsByTx[int(txNum)], nil
}

// MapProcessedTx satisfies the EventInfoMapper interface and always returns nil.
func (m staticMapper) MapProcessedTx(_ *fabric.ProcessedTransaction) ([]testEvent, error) {
	return nil, nil
}

// staticQueryService is a QueryByIDService stub that streams a fixed slice of
// events on each QueryByID call and records the parameters it received.
type staticQueryService struct {
	events         []testEvent
	err            error
	startingBlock  driver.BlockNum
	evictedLookups map[EventID][]ListenerEntry[testEvent]
}

// QueryByID returns a buffered channel pre-loaded with the configured events.
// It records startingBlock and evicted for post-call assertions. If err is set,
// QueryByID returns that error instead.
func (s *staticQueryService) QueryByID(_ context.Context, startingBlock driver.BlockNum, evicted map[EventID][]ListenerEntry[testEvent]) (<-chan []testEvent, error) {
	if s.err != nil {
		return nil, s.err
	}

	s.startingBlock = startingBlock
	s.evictedLookups = evicted
	ch := make(chan []testEvent, 1)
	ch <- s.events
	close(ch)
	return ch, nil
}

// recordingDelivery is a Delivery stub that captures the BlockCallback passed
// to ScanBlock so tests can inspect it after construction.
type recordingDelivery struct {
	called chan fabric.BlockCallback
}

// ScanBlock implements Delivery by forwarding callback to the called channel
// and returning nil, simulating a delivery service that starts successfully.
func (d *recordingDelivery) ScanBlock(_ context.Context, callback fabric.BlockCallback) error {
	d.called <- callback
	return nil
}

// newTestListenerManager creates a ListenerManager wired with in-memory caches
// and a no-op staticMapper for use in unit tests. It runs in sequential mode
// so block callbacks are deterministic and do not require synchronisation.
func newTestListenerManager(t *testing.T) *ListenerManager[testEvent] {
	t.Helper()

	logger := testLogger(t)
	return &ListenerManager[testEvent]{
		logger:             logger,
		mapper:             &parallelBlockMapper[testEvent]{logger: logger, cap: 1, mapper: staticMapper{}},
		listeners:          cache.NewMapCache[EventID, []ListenerEntry[testEvent]](),
		permanentListeners: map[EventID][]ListenerEntry[testEvent]{},
		events:             cache.NewMapCache[EventID, testEvent](),
		sequential:         true,
	}
}

// testLogger returns a named Logger suitable for use inside a test. The helper
// flag ensures that failure messages point to the calling test function rather
// than to this helper.
func testLogger(t *testing.T) logging.Logger {
	t.Helper()

	return logging.MustGetLogger("events", "test")
}

// waitForEvent blocks until an event is received on ch or the test-scoped
// timeout elapses, in which case the test is marked as failed immediately.
func waitForEvent(t *testing.T, ch <-chan testEvent) testEvent {
	t.Helper()

	select {
	case event := <-ch:
		return event
	case <-time.After(testListenerTimeout):
		t.Fatal("timed out waiting for listener callback")
		return testEvent{}
	}
}

// waitForCallback blocks until a BlockCallback is received on ch or the
// test-scoped timeout elapses, in which case the test is marked as failed.
func waitForCallback(t *testing.T, ch <-chan fabric.BlockCallback) fabric.BlockCallback {
	t.Helper()

	select {
	case callback := <-ch:
		return callback
	case <-time.After(testListenerTimeout):
		t.Fatal("timed out waiting for delivery callback")
		return nil
	}
}
