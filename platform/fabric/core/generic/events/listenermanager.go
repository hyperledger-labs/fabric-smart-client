/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type EventID = string

type EventInfo interface {
	ID() EventID
}

type EventInfoMapper[T EventInfo] interface {
	MapTxData(ctx context.Context, tx []byte, block *common.BlockMetadata, blockNum driver.BlockNum, txNum driver.TxNum) (map[driver.Namespace]T, error)
	MapProcessedTx(tx *fabric.ProcessedTransaction) ([]T, error)
}

type ListenerEntry[T EventInfo] interface {
	// Namespace returns the namespace this entry refers to. It can be empty.
	Namespace() driver.Namespace
	// OnStatus is the callback for the transaction
	OnStatus(ctx context.Context, info T)
	// Equals compares a listener entry for the delition
	Equals(other ListenerEntry[T]) bool
}

type QueryByIDService[T EventInfo] interface {
	QueryByID(ctx context.Context, startingBlock driver.BlockNum, evicted map[EventID][]ListenerEntry[T]) (<-chan []T, error)
}

type Delivery interface {
	ScanBlock(background context.Context, callback fabric.BlockCallback) error
}

type EventInfoCallback[T EventInfo] func(T) error

type DeliveryListenerManagerConfig struct {
	MapperParallelism       int
	BlockProcessParallelism int
	ListenerTimeout         time.Duration
	LRUSize                 int
	LRUBuffer               int
}

type ListenerManager[T EventInfo] struct {
	logger logging.Logger
	tracer trace.Tracer
	mapper *parallelBlockMapper[T]

	lastBlockNum               atomic.Uint64
	mu                         sync.RWMutex
	listeners                  cache.Map[EventID, []ListenerEntry[T]]
	permanentListeners         map[EventID][]ListenerEntry[T]
	events                     cache.Map[EventID, T]
	delivery                   Delivery
	blockProcessingParallelism int
	ignoreBlockErrors          bool
}

func NewListenerManager[T EventInfo](
	logger logging.Logger,
	config DeliveryListenerManagerConfig,
	delivery Delivery,
	queryService QueryByIDService[T],
	tracer trace.Tracer,
	mapper EventInfoMapper[T],
) (*ListenerManager[T], error) {
	var events cache.Map[EventID, T]
	if config.LRUSize > 0 && config.LRUBuffer > 0 {
		events = cache.NewLRUCache[EventID, T](config.LRUSize, config.LRUBuffer, func(evicted map[EventID]T) {
			logger.Debugf("evicted keys [%s]. If they are looked up, they will be fetched directly from the ledger from now on...", logging.Keys(evicted))
		})
	} else {
		events = cache.NewMapCache[EventID, T]()
	}
	flm := &ListenerManager[T]{
		logger:                     logger,
		mapper:                     &parallelBlockMapper[T]{logger: logger, cap: max(config.MapperParallelism, 1), mapper: mapper},
		tracer:                     tracer,
		events:                     events,
		ignoreBlockErrors:          true,
		blockProcessingParallelism: config.BlockProcessParallelism,
		delivery:                   delivery,
		permanentListeners:         make(map[EventID][]ListenerEntry[T]),
	}
	var listeners cache.Map[EventID, []ListenerEntry[T]]
	if config.ListenerTimeout > 0 {
		listeners = cache.NewTimeoutCache[EventID, []ListenerEntry[T]](config.ListenerTimeout, func(evicted map[EventID][]ListenerEntry[T]) {
			if len(evicted) == 0 {
				return
			}
			lastBlockNum := flm.lastBlockNum.Load()
			logger.Warnf(
				"listeners for TXs [%s] timed out. Last Block Num [%d]. Either the TX finality is too slow or it reached finality too long ago and were evicted from the events cache. The IDs will be queried directly from ledger (when the batch is cut)...",
				logging.Keys(evicted),
				lastBlockNum,
			)
			fetchTxs(logger, context.TODO(), lastBlockNum, evicted, queryService)
		})
	} else {
		listeners = cache.NewMapCache[EventID, []ListenerEntry[T]]()
	}
	flm.listeners = listeners

	logger.Debugf("starting delivery service...")
	go flm.start()

	return flm, nil
}

func (m *ListenerManager[T]) start() {
	// In case the delivery service fails, it will try to reconnect automatically.
	err := m.delivery.ScanBlock(context.Background(), m.newBlockCallback())
	m.logger.Errorf("failed running delivery: %v", err)
}

func (m *ListenerManager[T]) newBlockCallback() fabric.BlockCallback {
	if m.blockProcessingParallelism <= 1 {
		return func(ctx context.Context, block *common.Block) (bool, error) {
			err := m.onBlock(ctx, block)
			return !m.ignoreBlockErrors && err != nil, err
		}
	}
	eg := errgroup.Group{}
	eg.SetLimit(m.blockProcessingParallelism)
	return func(ctx context.Context, block *common.Block) (bool, error) {
		eg.Go(func() error {
			if err := m.onBlock(ctx, block); err != nil {
				m.logger.Warnf("Mapping block [%d] errored: %v", block.Header.Number, err)
			}
			return nil
		})
		return false, nil
	}
}

func (m *ListenerManager[T]) onBlock(ctx context.Context, block *common.Block) error {
	m.logger.Debugf("new block with %d buckets detected [%d]", len(block.Data.Data), block.Header.Number)

	buckets, err := m.mapper.Map(ctx, block)
	if err != nil {
		m.logger.Errorf("failed to process block [%d]: %v", block.Header.Number, err)
		return errors.Wrapf(err, "failed to process block [%d]", block.Header.Number)
	}

	invokedEventIDs := make([]EventID, 0)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastBlockNum.Store(block.Header.Number)

	for _, events := range buckets {
		for ns, event := range events {
			m.logger.Debugf("look for listeners of [%s:%s]", ns, event.ID())
			// We expect there to be only one namespace.
			// The complexity is better with a deliveryListenerEntry slice (because of the write operations)
			// If more namespaces are expected, it is worth switching to a map.
			listeners, ok := m.listeners.Get(event.ID())
			if ok {
				invokedEventIDs = append(invokedEventIDs, event.ID())
			}
			pListeners := m.permanentListeners[event.ID()]
			listeners = append(listeners, pListeners...)

			m.logger.Debugf("invoking [%d] listeners and [%d] permanent listeners for [%s]", len(listeners), len(pListeners), event.ID())
			for _, entry := range listeners {
				go entry.OnStatus(ctx, event)
			}
		}
	}
	m.logger.Debugf("invoked listeners for %d EventIDs: [%v]. Removing listeners...", len(invokedEventIDs), invokedEventIDs)

	for _, events := range buckets {
		for ns, event := range events {
			m.logger.Debugf("Mapping for ns [%s]", ns)
			m.events.Put(event.ID(), event)
		}
	}
	m.logger.Debugf("current size of cache: %d", m.events.Len())

	m.listeners.Delete(invokedEventIDs...)
	m.logger.Debugf("removed listeners for %d invoked EventIDs: %v", len(invokedEventIDs), invokedEventIDs)

	return nil
}

func (m *ListenerManager[T]) AddEventListener(id EventID, e ListenerEntry[T]) error {
	m.mu.RLock()
	if event, ok := m.events.Get(id); ok {
		defer m.mu.RUnlock()
		m.logger.Debugf("found event [%s]. Invoking listener directly", id)
		go e.OnStatus(context.TODO(), event)
		return nil
	}
	m.mu.RUnlock()
	m.mu.Lock()
	m.logger.Debugf("checking if value has been added meanwhile for [%s]", id)
	defer m.mu.Unlock()
	if event, ok := m.events.Get(id); ok {
		m.logger.Debugf("found event [%s]! Invoking listener directly", id)
		go e.OnStatus(context.TODO(), event)
		return nil
	}
	m.logger.Debugf("value not found. Appending listener for [%s]", id)
	m.listeners.Update(id, func(_ bool, listeners []ListenerEntry[T]) (bool, []ListenerEntry[T]) {
		return true, append(listeners, e)
	})
	return nil
}

func (m *ListenerManager[T]) AddPermanentEventListener(id EventID, e ListenerEntry[T]) error {
	m.mu.Lock()
	m.logger.Debugf("checking if value has been added meanwhile for [%s]", id)
	defer m.mu.Unlock()
	if event, ok := m.events.Get(id); ok {
		m.logger.Debugf("found event [%s]! Invoking listener directly", id)
		go e.OnStatus(context.TODO(), event)
	}
	m.logger.Debugf("appending permanent listener for [%s]", id)
	pListeners := m.permanentListeners[id]
	pListeners = append(pListeners, e)
	m.permanentListeners[id] = pListeners
	return nil
}

func (m *ListenerManager[T]) RemoveEventListener(id EventID, e ListenerEntry[T]) error {
	m.logger.Debugf("manually invoked listener removal for [%s]", id)
	m.mu.Lock()
	defer m.mu.Unlock()
	ok := m.listeners.Update(id, func(_ bool, listeners []ListenerEntry[T]) (bool, []ListenerEntry[T]) {
		for i, entry := range listeners {
			if entry.Equals(e) {
				listeners = append(listeners[:i], listeners[i+1:]...)
			}
		}
		return len(listeners) > 0, listeners
	})
	if ok {
		return nil
	}
	return errors.Errorf("could not find listener [%v] in eventID [%s]", e, id)
}

func fetchTxs[T EventInfo](logger logging.Logger, ctx context.Context, lastBlock driver.BlockNum, evicted map[EventID][]ListenerEntry[T], queryService QueryByIDService[T]) {
	go func() {
		ch, err := queryService.QueryByID(ctx, lastBlock, evicted)
		if err != nil {
			logger.Errorf("failed scanning: %v", err)
			return
		}
		for events := range ch {
			logger.Debugf("received [%d] events", len(events))
			for _, event := range events {
				logger.Debugf("evicted event [%s], notify [%d] listeners", event.ID(), len(evicted[event.ID()]))
				for i, listener := range evicted[event.ID()] {
					logger.Debugf("calling listener [%d], event [%s]", i, event.ID())
					go listener.OnStatus(context.TODO(), event)
				}
			}
		}
	}()
}

type parallelBlockMapper[T EventInfo] struct {
	logger logging.Logger
	mapper EventInfoMapper[T]
	cap    int
}

func (m *parallelBlockMapper[T]) Map(ctx context.Context, block *common.Block) ([]map[driver.Namespace]T, error) {
	m.logger.Debugf("mapping block [%d]", block.Header.Number)
	eg := errgroup.Group{}
	eg.SetLimit(m.cap)
	results := make([]map[driver.Namespace]T, len(block.Data.Data))
	for i, tx := range block.Data.Data {
		eg.Go(func() error {
			event, err := m.mapper.MapTxData(ctx, tx, block.Metadata, block.Header.Number, driver.TxNum(i))
			if err != nil {
				return err
			}
			results[i] = event
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}
