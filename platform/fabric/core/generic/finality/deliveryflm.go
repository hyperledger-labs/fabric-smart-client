/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"sync"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type TxInfo interface {
	TxID() driver2.TxID
}

type TxInfoMapper[T TxInfo] interface {
	Map(ctx context.Context, tx []byte, block *common.BlockMetadata, blockNum driver2.BlockNum, txNum driver2.TxNum) (map[driver2.Namespace]T, error)
}

type ListenerEntry[T TxInfo] interface {
	// OnStatus is the callback for the transaction
	OnStatus(ctx context.Context, info T)
	// Equals compares a listener entry for the delition
	Equals(other ListenerEntry[T]) bool
}

type ListenerManager[T TxInfo] interface {
	AddFinalityListener(txID string, e ListenerEntry[T]) error
	RemoveFinalityListener(txID string, e ListenerEntry[T]) error
}

type ListenerManagerProvider[T TxInfo] interface {
	NewManager(network, channel string, mapper TxInfoMapper[T]) (ListenerManager[T], error)
}

// deliveryBasedFLMProvider assumes that a listener for a transaction is added before the transaction (i.e. the corresponding block) arrives in the delivery service listener.
type deliveryBasedFLMProvider[T TxInfo] struct {
	fnsp           *fabric.NetworkServiceProvider
	tracerProvider trace.TracerProvider
}

func NewDeliveryBasedFLMProvider[T TxInfo](fnsp *fabric.NetworkServiceProvider, tracerProvider trace.TracerProvider) *deliveryBasedFLMProvider[T] {
	return &deliveryBasedFLMProvider[T]{
		fnsp:           fnsp,
		tracerProvider: tracerProvider,
	}
}

func (p *deliveryBasedFLMProvider[T]) NewManager(network, channel string, mapper TxInfoMapper[T]) (ListenerManager[T], error) {
	net, err := p.fnsp.FabricNetworkService(network)
	if err != nil {
		return nil, err
	}
	ch, err := net.Channel(channel)
	if err != nil {
		return nil, err
	}

	return NewDeliveryBasedFLM(ch, p.tracerProvider.Tracer("finality_listener_manager", tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace: network,
	})), mapper)
}

func NewDeliveryBasedFLM[T TxInfo](ch *fabric.Channel, tracer trace.Tracer, mapper TxInfoMapper[T]) (*deliveryBasedFLM[T], error) {
	flm := &deliveryBasedFLM[T]{
		mapper:    NewParallelResponseMapper[T](10, mapper),
		tracer:    tracer,
		listeners: cache.NewMapCache[driver2.TxID, []ListenerEntry[T]](),
		txInfos:   cache.NewMapCache[driver2.TxID, T](),
	}
	logger.Infof("Starting delivery service for [%s]", ch.Name())
	go func() {
		err := ch.Delivery().ScanBlock(context.Background(), func(ctx context.Context, block *common.Block) (bool, error) {
			return false, flm.onBlock(ctx, block)
		})
		logger.Errorf("failed running delivery for [%s]: %v", ch.Name(), err)
	}()

	return flm, nil
}

type deliveryBasedFLM[T TxInfo] struct {
	tracer trace.Tracer
	mapper *parallelBlockMapper[T]

	mu        sync.RWMutex
	listeners cache.Map[driver2.TxID, []ListenerEntry[T]]
	txInfos   cache.Map[driver2.TxID, T]
}

func (m *deliveryBasedFLM[T]) onBlock(ctx context.Context, block *common.Block) error {
	logger.Infof("New block with %d txs detected [%d]", len(block.Data.Data), block.Header.Number)

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
			logger.Infof("Look for listeners of [%s:%s]", ns, info.TxID())
			// We expect there to be only one namespace.
			// The complexity is better with a listenerEntry slice (because of the write operations)
			// If more namespaces are expected, it is worth switching to a map.
			listeners, ok := m.listeners.Get(info.TxID())
			if ok {
				invokedTxIDs = append(invokedTxIDs, info.TxID())
			}
			logger.Infof("Invoking %d listeners for [%s]", len(listeners), info.TxID())
			for _, entry := range listeners {
				go entry.OnStatus(ctx, info)
			}
		}
	}
	//m.mu.RUnlock()

	logger.Infof("Invoked listeners for %d TxIDs: [%v]. Removing listeners...", len(invokedTxIDs), invokedTxIDs)

	//m.mu.Lock()
	//defer m.mu.Unlock()
	for _, txInfos := range txs {
		for ns, info := range txInfos {
			logger.Warnf("Mapping for ns [%s]", ns)
			m.txInfos.Put(info.TxID(), info)
		}
	}
	logger.Infof("Current size of cache: %d", m.txInfos.Len())

	m.listeners.Delete(invokedTxIDs...)

	logger.Infof("Removed listeners for %d invoked TxIDs: %v", len(invokedTxIDs), invokedTxIDs)

	return nil

}

func (m *deliveryBasedFLM[T]) AddFinalityListener(txID string, e ListenerEntry[T]) error {
	m.mu.RLock()
	if txInfo, ok := m.txInfos.Get(txID); ok {
		defer m.mu.RUnlock()
		logger.Infof("Found tx [%s]. Invoking listener directly", txID)
		go e.OnStatus(context.TODO(), txInfo)
		return nil
	}
	m.mu.RUnlock()
	m.mu.Lock()
	logger.Infof("Checking if value has been added meanwhile for [%s]", txID)
	defer m.mu.Unlock()
	if txInfo, ok := m.txInfos.Get(txID); ok {
		logger.Infof("Found tx [%s]! Invoking listener directly", txID)
		go e.OnStatus(context.TODO(), txInfo)
		return nil
	}
	m.listeners.Update(txID, func(_ bool, listeners []ListenerEntry[T]) (bool, []ListenerEntry[T]) {
		return true, append(listeners, e)
	})
	return nil
}

func (m *deliveryBasedFLM[T]) RemoveFinalityListener(txID string, e ListenerEntry[T]) error {
	logger.Infof("Manually invoked listener removal for [%s]", txID)
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

func NewParallelResponseMapper[T TxInfo](cap int, mapper TxInfoMapper[T]) *parallelBlockMapper[T] {
	return &parallelBlockMapper[T]{cap: cap, mapper: mapper}
}

func (m *parallelBlockMapper[T]) Map(ctx context.Context, block *common.Block) ([]map[driver2.Namespace]T, error) {
	logger.Infof("Mapping block [%d]", block.Header.Number)
	eg := errgroup.Group{}
	eg.SetLimit(m.cap)
	results := make([]map[driver2.Namespace]T, len(block.Data.Data))
	for i, tx := range block.Data.Data {
		eg.Go(func() error {
			event, err := m.mapper.Map(ctx, tx, block.Metadata, block.Header.Number, driver2.TxNum(i))
			if err != nil {
				return err
			}
			results[i] = event
			logger.Infof("Put tx [%d:%d]: [%v]", block.Header.Number, i, event)
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}
