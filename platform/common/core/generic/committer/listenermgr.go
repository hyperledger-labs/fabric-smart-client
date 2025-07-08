/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type finalityListenerManagerProvider[V comparable] struct {
	logger Logger
	tracer trace.Tracer
}

func NewFinalityListenerManagerProvider[V comparable](tracerProvider tracing.Provider) *finalityListenerManagerProvider[V] {
	return &finalityListenerManagerProvider[V]{
		logger: logging.MustGetLogger(),
		tracer: tracerProvider.Tracer("finality_listener_manager", tracing.WithMetricsOpts(tracing.MetricsOpts{Namespace: "core"})),
	}
}

func (p *finalityListenerManagerProvider[V]) NewManager() driver.ListenerManager[V] {
	return newFinalityListenerManager[V](p.logger, p.tracer)
}

type finalityListenerManager[V comparable] struct {
	mutex         sync.RWMutex
	logger        Logger
	tracer        trace.Tracer
	txIDListeners map[driver.TxID][]driver.FinalityListener[V]
}

func newFinalityListenerManager[V comparable](logger Logger, tracer trace.Tracer) *finalityListenerManager[V] {
	return &finalityListenerManager[V]{
		logger:        logger,
		tracer:        tracer,
		txIDListeners: make(map[driver.TxID][]driver.FinalityListener[V]),
	}
}

func (c *finalityListenerManager[V]) AddListener(txID driver.TxID, toAdd driver.FinalityListener[V]) error {
	if len(txID) == 0 {
		return errors.Errorf("tx id must be not empty")
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.txIDListeners[txID]
	if !ok {
		ls = []driver.FinalityListener[V]{}
	}
	c.txIDListeners[txID] = append(ls, toAdd)

	return nil
}

func (c *finalityListenerManager[V]) RemoveListener(txID driver.TxID, toRemove driver.FinalityListener[V]) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if ls, ok := collections.Remove(c.txIDListeners[txID], toRemove); ok {
		c.txIDListeners[txID] = ls
		if len(ls) == 0 {
			delete(c.txIDListeners, txID)
		}
	}
}

func (c *finalityListenerManager[V]) InvokeListeners(event driver.FinalityEvent[V]) {
	newCtx, span := c.tracer.Start(event.Ctx, "dispatch")
	defer span.End()
	listeners := c.cloneListeners(event.TxID)
	// c.logger.Debugf("dispatch event [%s][%d][%d]", event.TxID, event.Code, len(listeners))
	span.AddEvent("dispatch_to_listeners")
	for _, listener := range listeners {
		span.AddEvent("invoke_listener")
		c.invokeListener(newCtx, listener, event.TxID, event.ValidationCode, event.ValidationMessage)
	}
}

func (c *finalityListenerManager[V]) invokeListener(ctx context.Context, l driver.FinalityListener[V], txID driver.TxID, status V, statusMessage string) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Errorf("caught panic while running dispatching event [%s:%d:%s]: [%s][%s]", txID, status, statusMessage, r, debug.Stack())
		}
	}()
	l.OnStatus(ctx, txID, status, statusMessage)
}

func (c *finalityListenerManager[V]) cloneListeners(txID driver.TxID) []driver.FinalityListener[V] {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	txListeners := c.txIDListeners[txID]
	clone := make([]driver.FinalityListener[V], len(txListeners))
	copy(clone, txListeners)
	delete(c.txIDListeners, txID)

	return clone
}

func (c *finalityListenerManager[V]) TxIDs() []driver.TxID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return collections.Keys(c.txIDListeners)
}
