/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
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
	DebugfContext(ctx context.Context, template string, args ...interface{})
	Infof(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
}

type Vault[V comparable] interface {
	Statuses(ctx context.Context, ids ...driver.TxID) ([]driver.TxValidationStatus[V], error)
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
	tracer            trace.Tracer
	eventQueueWorkers int
}

func NewFinalityManager[V comparable](listenerManager driver.ListenerManager[V], logger Logger, vault Vault[V], tracerProvider tracing.Provider, eventQueueWorkers int, statuses ...V) *FinalityManager[V] {
	return &FinalityManager[V]{
		listenerManager: listenerManager,
		logger:          logger,
		eventQueue:      make(chan driver.FinalityEvent[V], defaultEventQueueSize),
		vault:           vault,
		postStatuses:    collections.NewSet(statuses...),
		tracer: tracerProvider.Tracer("finality_manager", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace: "core",
		})),
		eventQueueWorkers: eventQueueWorkers,
	}
}

func (c *FinalityManager[V]) AddListener(txID driver.TxID, toAdd driver.FinalityListener[V]) error {
	return c.listenerManager.AddListener(txID, toAdd)
}

func (c *FinalityManager[V]) RemoveListener(txID driver.TxID, toRemove driver.FinalityListener[V]) {
	c.listenerManager.RemoveListener(txID, toRemove)
}

func (c *FinalityManager[V]) Post(event driver.FinalityEvent[V]) {
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
			txIDs := c.listenerManager.TxIDs()
			if len(txIDs) == 0 {
				c.logger.Debugf("no transactions to check vault status")
				break
			}

			newCtx, span := c.tracer.Start(context.Background(), "committer_status_listener")
			c.logger.DebugfContext(ctx, "check vault status for [%d] transactions", len(txIDs))
			statuses, err := c.vault.Statuses(newCtx, txIDs...)
			if err != nil {
				c.logger.Errorf("error fetching statuses: %s", err.Error())
				span.RecordError(err)
				span.End()
				continue
			}
			c.logger.DebugfContext(ctx, "got vault status for [%d] transactions, post event...", len(txIDs))
			span.AddEvent("post_events")
			for _, status := range statuses {
				// check txID status, if it is valid or invalid, post an event
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
