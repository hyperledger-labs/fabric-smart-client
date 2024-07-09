/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const (
	// How often to poll the vault for newly-committed transactions
	checkVaultFrequency   = 1 * time.Second
	defaultEventQueueSize = 1000
)

type Logger interface {
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}

// FinalityEvent contains information about the finality of a given transaction
type FinalityEvent[V comparable] struct {
	Ctx               context.Context
	TxID              driver.TxID
	ValidationCode    V
	ValidationMessage string
	Block             driver.BlockNum
	IndexInBlock      driver.TxNum
	Err               error
}

type Vault[V comparable] interface {
	Statuses(ids ...string) ([]driver.TxValidationStatus[V], error)
}

// FinalityManager manages events for the commit pipeline.
// It consists of a central queue of events.
// The queue is fed by multiple sources.
// A single thread reads from this queue and invokes the listeners in a blocking way
type FinalityManager[V comparable] struct {
	logger            Logger
	eventQueue        chan FinalityEvent[V]
	vault             Vault[V]
	postStatuses      collections.Set[V]
	txIDListeners     map[driver.TxID][]driver.FinalityListener[V]
	tracer            trace.Tracer
	mutex             sync.RWMutex
	eventQueueWorkers int
}

func NewFinalityManager[V comparable](logger Logger, vault Vault[V], tracerProvider trace.TracerProvider, statuses ...V) *FinalityManager[V] {
	return &FinalityManager[V]{
		logger:        logger,
		eventQueue:    make(chan FinalityEvent[V], defaultEventQueueSize),
		vault:         vault,
		postStatuses:  collections.NewSet(statuses...),
		txIDListeners: map[string][]driver.FinalityListener[V]{},
		tracer: tracerProvider.Tracer("finality_manager", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace: "core",
		})),
		eventQueueWorkers: 300,
	}
}

func (c *FinalityManager[V]) AddListener(txID driver.TxID, toAdd driver.FinalityListener[V]) error {
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

func (c *FinalityManager[V]) RemoveListener(txID driver.TxID, toRemove driver.FinalityListener[V]) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if ls, ok := collections.Remove(c.txIDListeners[txID], toRemove); ok {
		c.txIDListeners[txID] = ls
		if len(ls) == 0 {
			delete(c.txIDListeners, txID)
		}
	}
}

func (c *FinalityManager[V]) Post(event FinalityEvent[V]) {
	c.logger.Debugf("post event [%s][%d]", event.TxID, event.ValidationCode)
	c.eventQueue <- event
}

func (c *FinalityManager[V]) Dispatch(event FinalityEvent[V]) {
	newCtx, span := c.tracer.Start(event.Ctx, "dispatch")
	defer span.End()
	listeners := c.cloneListeners(event.TxID)
	c.logger.Debugf("dispatch event [%s][%d][%d]", event.TxID, event.ValidationCode, len(listeners))
	span.AddEvent("dispatch_to_listeners")
	for _, listener := range listeners {
		span.AddEvent("invoke_listener")
		c.invokeListener(newCtx, listener, event.TxID, event.ValidationCode, event.ValidationMessage)
	}
}

func (c *FinalityManager[V]) Run(context context.Context) {
	for i := 0; i < c.eventQueueWorkers; i++ {
		go c.runEventQueue(context)
	}
	go c.runStatusListener(context)
}

func (c *FinalityManager[V]) invokeListener(ctx context.Context, l driver.FinalityListener[V], txID driver.TxID, status V, statusMessage string) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Errorf("caught panic while running dispatching event [%s:%d:%s]: [%s][%s]", txID, status, statusMessage, r, debug.Stack())
		}
	}()
	l.OnStatus(ctx, txID, status, statusMessage)
}

func (c *FinalityManager[V]) runEventQueue(context context.Context) {
	for {
		select {
		case <-context.Done():
			return
		case event := <-c.eventQueue:
			c.Dispatch(event)
		}
	}
}

func (c *FinalityManager[V]) runStatusListener(ctx context.Context) {
	ticker := time.NewTicker(checkVaultFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			txIDs := c.txIDs()
			if len(txIDs) == 0 {
				c.logger.Debugf("no transactions to check vault status")
				break
			}

			newCtx, span := c.tracer.Start(context.Background(), "vault_status_check")
			c.logger.Debugf("check vault status for [%d] transactions [%v]", len(txIDs), txIDs)
			statuses, err := c.vault.Statuses(txIDs...)
			if err != nil {
				c.logger.Errorf("error fetching statuses: %w", err)
				span.RecordError(err)
				span.End()
				continue
			}
			c.logger.Debugf("got vault status for [%d] transactions [%v], post event...", len(txIDs), txIDs)
			span.AddEvent("post_events")
			for _, status := range statuses {
				// check txID status, if it is valid or invalid, post an event
				c.logger.Debugf("check tx [%s]'s status [%v]", status.TxID, status.ValidationCode)
				if c.postStatuses.Contains(status.ValidationCode) {
					// post the event
					c.Post(FinalityEvent[V]{
						Ctx:               newCtx,
						TxID:              status.TxID,
						ValidationCode:    status.ValidationCode,
						ValidationMessage: status.Message,
					})
				}
			}
			span.End()
		}
	}
}

func (c *FinalityManager[V]) cloneListeners(txID driver.TxID) []driver.FinalityListener[V] {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	txListeners := c.txIDListeners[txID]
	clone := make([]driver.FinalityListener[V], len(txListeners))
	copy(clone, txListeners)
	delete(c.txIDListeners, txID)

	return clone
}

func (c *FinalityManager[V]) txIDs() []driver.TxID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return collections.Keys(c.txIDListeners)
}
