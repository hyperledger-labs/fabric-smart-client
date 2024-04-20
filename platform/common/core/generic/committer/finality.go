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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const (
	// How often to poll the vault for newly-committed transactions
	checkVaultFrequency = 1 * time.Second
	// Listeners that do not listen for a specific txID, but for all transactions
	allListenersKey       = ""
	defaultEventQueueSize = 1000
)

var logger = flogging.MustGetLogger("common-sdk.Committer")

// FinalityEvent contains information about the finality of a given transaction
type FinalityEvent[V comparable] struct {
	TxID              core.TxID
	ValidationCode    V
	ValidationMessage string
	Block             uint64
	IndexInBlock      int
	Err               error
}

// FinalityListener is the interface that must be implemented to receive transaction status notifications
type FinalityListener[V comparable] interface {
	// OnStatusChange is called when the status of a transaction changes, or it is valid or invalid
	OnStatusChange(txID core.TxID, status V, statusMessage string) error
}

type Vault[V comparable] interface {
	Statuses(ids ...string) ([]driver.TxValidationStatus[V], error)
}

// FinalityManager manages events for the commit pipeline.
// It consists of a central queue of events.
// The queue is fed by multiple sources.
// A single thread reads from this queue and invokes the listeners in a blocking way
type FinalityManager[V comparable] struct {
	eventQueue    chan FinalityEvent[V]
	vault         Vault[V]
	postStatuses  utils.Set[V]
	txIDListeners map[core.TxID][]FinalityListener[V]
	mutex         sync.RWMutex
}

func NewFinalityManager[V comparable](vault Vault[V], statuses ...V) *FinalityManager[V] {
	return &FinalityManager[V]{
		eventQueue:    make(chan FinalityEvent[V], defaultEventQueueSize),
		vault:         vault,
		postStatuses:  utils.NewSet(statuses...),
		txIDListeners: map[string][]FinalityListener[V]{},
	}
}

func (c *FinalityManager[V]) AddListener(txID core.TxID, toAdd FinalityListener[V]) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.txIDListeners[txID]
	if !ok {
		ls = []FinalityListener[V]{}
	}
	c.txIDListeners[txID] = append(ls, toAdd)
}

func (c *FinalityManager[V]) RemoveListener(txID core.TxID, toRemove FinalityListener[V]) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if ls, ok := utils.Remove(c.txIDListeners[txID], toRemove); ok {
		c.txIDListeners[txID] = ls
		if len(ls) == 0 {
			delete(c.txIDListeners, txID)
		}
	}
}

func (c *FinalityManager[V]) Post(event FinalityEvent[V]) {
	c.eventQueue <- event
}

func (c *FinalityManager[V]) Dispatch(event FinalityEvent[V]) {
	listeners := c.cloneListeners(event.TxID)
	for _, listener := range listeners {
		c.invokeListener(listener, event.TxID, event.ValidationCode, event.ValidationMessage)
	}
}

func (c *FinalityManager[V]) Run(context context.Context) {
	go c.runEventQueue(context)
	go c.runStatusListener(context)
}

func (c *FinalityManager[V]) invokeListener(l FinalityListener[V], txID core.TxID, status V, statusMessage string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("caught panic while running dispatching event [%s:%d:%s]: [%s][%s]", txID, status, statusMessage, r, debug.Stack())
		}
	}()
	l.OnStatusChange(txID, status, statusMessage)
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

func (c *FinalityManager[V]) runStatusListener(context context.Context) {
	ticker := time.NewTicker(checkVaultFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-context.Done():
			return
		case <-ticker.C:
			statuses, err := c.vault.Statuses(c.txIDs()...)
			if err != nil {
				logger.Errorf("error fetching statuses: %w", err)
				continue
			}
			for _, status := range statuses {
				// check txID status, if it is valid or invalid, post an event
				logger.Debugf("check tx [%s]'s status", status.TxID)
				if c.postStatuses.Contains(status.ValidationCode) {
					// post the event
					c.Post(FinalityEvent[V]{
						TxID:              status.TxID,
						ValidationCode:    status.ValidationCode,
						ValidationMessage: status.Message,
					})
				}
			}
		}
	}
}

func (c *FinalityManager[V]) cloneListeners(txID core.TxID) []FinalityListener[V] {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	txListeners := c.txIDListeners[txID]
	allListeners := c.txIDListeners[allListenersKey]
	clone := make([]FinalityListener[V], len(txListeners)+len(allListeners))
	copy(clone[:len(txListeners)], txListeners)
	copy(clone[len(txListeners):], allListeners)
	delete(c.txIDListeners, txID)

	return clone
}

func (c *FinalityManager[V]) txIDs() []core.TxID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	keys, _ := utils.Remove(utils.Keys(c.txIDListeners), allListenersKey)
	return keys
}