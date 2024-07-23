/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
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

type Vault[V comparable] interface {
	Statuses(ids ...string) ([]driver.TxValidationStatus[V], error)
}

// FinalityManager manages events for the commit pipeline.
// It consists of a central queue of events.
// The queue is fed by multiple sources.
// A single thread reads from this queue and invokes the listeners in a blocking way
type FinalityManager[V comparable] struct {
	listenerManager   driver.ListenerManager[V]
	logger            Logger
	eventQueue        chan driver.FinalityEvent[V]
	vault             Vault[V]
	postStatuses      collections.Set[V]
	txIDs             collections.Set[string]
	tracer            trace.Tracer
	mutex             sync.RWMutex
	eventQueueWorkers int
}

func NewFinalityManager[V comparable](listenerManager driver.ListenerManager[V], logger Logger, vault Vault[V], tracerProvider trace.TracerProvider, eventQueueWorkers int, statuses ...V) *FinalityManager[V] {
	return &FinalityManager[V]{
		listenerManager: listenerManager,
		logger:          logger,
		eventQueue:      make(chan driver.FinalityEvent[V], defaultEventQueueSize),
		vault:           vault,
		postStatuses:    collections.NewSet(statuses...),
		txIDs:           collections.NewSet[string](),
		tracer: tracerProvider.Tracer("finality_manager", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace: "core",
		})),
		eventQueueWorkers: eventQueueWorkers,
	}
}

func (c *FinalityManager[V]) AddListener(txID driver.TxID, toAdd driver.FinalityListener[V]) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if err := c.listenerManager.AddListener(txID, toAdd); err != nil {
		return err
	}
	c.txIDs.Add(txID)
	return nil
}

func (c *FinalityManager[V]) RemoveListener(txID driver.TxID, toRemove driver.FinalityListener[V]) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.txIDs.Remove(txID)
	c.listenerManager.RemoveListener(txID, toRemove)
}

func (c *FinalityManager[V]) Post(event driver.FinalityEvent[V]) {
	c.logger.Debugf("post event [%s][%d]", event.TxID, event.ValidationCode)
	c.eventQueue <- event
}

func (c *FinalityManager[V]) Run(context context.Context) {
	for i := 0; i < c.eventQueueWorkers; i++ {
		go c.runEventQueue(context)
	}
	go c.runStatusListener(context)
}

func (c *FinalityManager[V]) runEventQueue(context context.Context) {
	for {
		select {
		case <-context.Done():
			return
		case event := <-c.eventQueue:
			c.listenerManager.InvokeListeners(event)
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
			c.mutex.RLock()
			txIDs := c.txIDs.ToSlice()
			c.mutex.RUnlock()
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
					c.Post(driver.FinalityEvent[V]{
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

//func (c *FinalityManager[V]) txIDs() []driver.TxID {
//	c.mutex.RLock()
//	defer c.mutex.RUnlock()
//
//	return collections.Keys(c.txIDListeners)
//}
