/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"sync"
	"time"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type TxInfo interface {
	TxID() driver2.TxID
}

type TxInfoMapper[T TxInfo] interface {
	MapTxData(ctx context.Context, tx []byte, block *common.BlockMetadata, blockNum driver2.BlockNum, txNum driver2.TxNum) (map[driver2.Namespace]T, error)
	MapProcessedTx(tx *fabric.ProcessedTransaction) ([]T, error)
}

type ListenerEntry[T TxInfo] interface {
	Namespace() driver2.Namespace
	// OnStatus is the callback for the transaction
	OnStatus(ctx context.Context, info T)
	// Equals compares a listener entry for the delition
	Equals(other ListenerEntry[T]) bool
}

type ListenerManager[T TxInfo] interface {
	AddFinalityListener(txID string, e ListenerEntry[T]) error
	RemoveFinalityListener(txID string, e ListenerEntry[T]) error
}

type QueryByIDService[T TxInfo] interface {
	QueryByID(ids ...driver2.TxID) (<-chan []T, error)
}

type TxInfoCallback[T TxInfo] func(T) error

type DeliveryListenerManagerConfig struct {
	MapperParallelism       int
	BlockProcessParallelism int
	ListenerTimeout         time.Duration
	LRUSize                 int
	LRUBuffer               int
}

func NewListenerManager[T TxInfo](
	config DeliveryListenerManagerConfig,
	delivery *fabric.Delivery,
	queryService QueryByIDService[T],
	tracer trace.Tracer,
	mapper TxInfoMapper[T],
) (*listenerManager[T], error) {
	var listeners cache.Map[driver2.TxID, []ListenerEntry[T]]
	if config.ListenerTimeout > 0 {
		listeners = cache.NewTimeoutCache[driver2.TxID, []ListenerEntry[T]](config.ListenerTimeout, func(evicted map[driver2.TxID][]ListenerEntry[T]) {
			if len(evicted) == 0 {
				return
			}
			logger.Warnf("Listeners for TXs [%v] timed out. Either the TX finality is too slow or it reached finality too long ago and were evicted from the txInfos cache. The IDs will be queried directly from ledger (when the batch is cut)...", collections.Keys(evicted))
			fetchTxs(evicted, queryService)
		})
	} else {
		listeners = cache.NewMapCache[driver2.TxID, []ListenerEntry[T]]()
	}

	var txInfos cache.Map[driver2.TxID, T]
	if config.LRUSize > 0 && config.LRUBuffer > 0 {
		txInfos = cache.NewLRUCache[driver2.TxID, T](10, 2, func(evicted map[driver2.TxID]T) {
			logger.Debugf("Evicted keys [%v]. If they are looked up, they will be fetched directly from the ledger from now on...", collections.Keys(evicted))
		})
	} else {
		txInfos = cache.NewMapCache[driver2.TxID, T]()
	}
	flm := &listenerManager[T]{
		mapper:                     &parallelBlockMapper[T]{cap: max(config.MapperParallelism, 1), mapper: mapper},
		tracer:                     tracer,
		listeners:                  listeners,
		txInfos:                    txInfos,
		ignoreBlockErrors:          true,
		blockProcessingParallelism: config.BlockProcessParallelism,
		delivery:                   delivery,
	}
	logger.Infof("Starting delivery service...")
	go flm.start()

	return flm, nil
}

func fetchTxs[T TxInfo](evicted map[driver2.TxID][]ListenerEntry[T], queryService QueryByIDService[T]) {
	go func() {
		keys := collections.Keys(evicted)
		logger.Debugf("Launching routine to scan for txs [%v]", keys)

		ch, err := queryService.QueryByID(keys...)
		if err != nil {
			logger.Errorf("Failed scanning: %v", err)
			return
		}
		for infos := range ch {
			for _, info := range infos {
				for _, listener := range evicted[info.TxID()] {
					go listener.OnStatus(context.TODO(), info)
				}
			}
		}
	}()
}

func (m *listenerManager[T]) start() {
	// In case the delivery service fails, it will try to reconnect automatically.
	err := m.delivery.ScanBlock(context.Background(), m.newBlockCallback())
	logger.Errorf("failed running delivery: %v", err)
}

func (m *listenerManager[T]) newBlockCallback() fabric.BlockCallback {
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
				logger.Warnf("Mapping block [%d] errored: %v", block.Header.Number, err)
			}
			return nil
		})
		return false, nil
	}
}

type listenerManager[T TxInfo] struct {
	tracer trace.Tracer
	mapper *parallelBlockMapper[T]

	mu                         sync.RWMutex
	listeners                  cache.Map[driver2.TxID, []ListenerEntry[T]]
	txInfos                    cache.Map[driver2.TxID, T]
	delivery                   *fabric.Delivery
	blockProcessingParallelism int
	ignoreBlockErrors          bool
}

func (m *listenerManager[T]) onBlock(ctx context.Context, block *common.Block) error {
	logger.Debugf("New block with %d txs detected [%d]", len(block.Data.Data), block.Header.Number)

	txs, err := m.mapper.Map(ctx, block)
	if err != nil {
		logger.Errorf("failed to process block [%d]: %v", block.Header.Number, err)
		return errors.Wrapf(err, "failed to process block [%d]", block.Header.Number)
	}

	invokedTxIDs := make([]driver2.TxID, 0)

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, txInfos := range txs {
		for ns, info := range txInfos {
			logger.Debugf("Look for listeners of [%s:%s]", ns, info.TxID())
			// We expect there to be only one namespace.
			// The complexity is better with a deliveryListenerEntry slice (because of the write operations)
			// If more namespaces are expected, it is worth switching to a map.
			listeners, ok := m.listeners.Get(info.TxID())
			if ok {
				invokedTxIDs = append(invokedTxIDs, info.TxID())
			}
			logger.Debugf("Invoking %d listeners for [%s]", len(listeners), info.TxID())
			for _, entry := range listeners {
				go entry.OnStatus(ctx, info)
			}
		}
	}
	logger.Debugf("Invoked listeners for %d TxIDs: [%v]. Removing listeners...", len(invokedTxIDs), invokedTxIDs)

	for _, txInfos := range txs {
		for ns, info := range txInfos {
			logger.Debugf("Mapping for ns [%s]", ns)
			m.txInfos.Put(info.TxID(), info)
		}
	}
	logger.Debugf("Current size of cache: %d", m.txInfos.Len())

	m.listeners.Delete(invokedTxIDs...)
	logger.Debugf("Removed listeners for %d invoked TxIDs: %v", len(invokedTxIDs), invokedTxIDs)

	return nil
}

func (m *listenerManager[T]) AddFinalityListener(txID string, e ListenerEntry[T]) error {
	m.mu.RLock()
	if txInfo, ok := m.txInfos.Get(txID); ok {
		defer m.mu.RUnlock()
		logger.Debugf("Found tx [%s]. Invoking listener directly", txID)
		go e.OnStatus(context.TODO(), txInfo)
		return nil
	}
	m.mu.RUnlock()
	m.mu.Lock()
	logger.Debugf("Checking if value has been added meanwhile for [%s]", txID)
	defer m.mu.Unlock()
	if txInfo, ok := m.txInfos.Get(txID); ok {
		logger.Debugf("Found tx [%s]! Invoking listener directly", txID)
		go e.OnStatus(context.TODO(), txInfo)
		return nil
	}
	logger.Debugf("Value not found. Appending listener for [%s]", txID)
	m.listeners.Update(txID, func(_ bool, listeners []ListenerEntry[T]) (bool, []ListenerEntry[T]) {
		return true, append(listeners, e)
	})
	return nil
}

func (m *listenerManager[T]) RemoveFinalityListener(txID string, e ListenerEntry[T]) error {
	logger.Debugf("Manually invoked listener removal for [%s]", txID)
	m.mu.Lock()
	defer m.mu.Unlock()
	ok := m.listeners.Update(txID, func(_ bool, listeners []ListenerEntry[T]) (bool, []ListenerEntry[T]) {
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
	return errors.Errorf("could not find listener [%v] in txid [%s]", e, txID)
}

type parallelBlockMapper[T TxInfo] struct {
	mapper TxInfoMapper[T]
	cap    int
}

func (m *parallelBlockMapper[T]) Map(ctx context.Context, block *common.Block) ([]map[driver2.Namespace]T, error) {
	logger.Debugf("Mapping block [%d]", block.Header.Number)
	eg := errgroup.Group{}
	eg.SetLimit(m.cap)
	results := make([]map[driver2.Namespace]T, len(block.Data.Data))
	for i, tx := range block.Data.Data {
		eg.Go(func() error {
			event, err := m.mapper.MapTxData(ctx, tx, block.Metadata, block.Header.Number, driver2.TxNum(i))
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
