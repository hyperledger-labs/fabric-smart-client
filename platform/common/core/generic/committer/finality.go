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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const checkVaultFrequency = 1 * time.Second

type TxID = string

var logger = flogging.MustGetLogger("common-sdk.Committer")

// FinalityEvent contains information about the finality of a given transaction
type FinalityEvent[V comparable] struct {
	TxID              TxID
	ValidationCode    V
	ValidationMessage string
	Block             uint64
	IndexInBlock      int
	Err               error
}

// FinalityListener is the interface that must be implemented to receive transaction status notifications
type FinalityListener[V comparable] interface {
	// OnStatus is called when the status of a transaction changes, or it is valid or invalid
	OnStatus(txID TxID, status V, statusMessage string)
}

type Vault[V comparable] interface {
	Statuses(ids ...string) ([]driver.TxValidationStatus[V], error)
}

// FinalityManager manages events for the commit pipeline.
// It consists of a central queue of events.
// The queue is fed by multiple sources.
// A single thread reads from this queue and invokes the listeners in a blocking way
type FinalityManager[V comparable] struct {
	EventQueue chan FinalityEvent[V]
	Vault      Vault[V]

	statuses      utils.Set[V]
	allListeners  []FinalityListener[V]
	txIDListeners map[TxID][]FinalityListener[V]
	mutex         sync.RWMutex
}

func NewFinalityManager[V comparable](vault Vault[V], size int, statuses ...V) *FinalityManager[V] {
	return &FinalityManager[V]{
		EventQueue:    make(chan FinalityEvent[V], size),
		Vault:         vault,
		statuses:      utils.NewSet(statuses...),
		allListeners:  []FinalityListener[V]{},
		txIDListeners: map[string][]FinalityListener[V]{},
	}
}

func (c *FinalityManager[V]) AddListener(txID TxID, toAdd FinalityListener[V]) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(txID) == 0 {
		c.allListeners = append(c.allListeners, toAdd)
	}

	ls, ok := c.txIDListeners[txID]
	if !ok {
		ls = []FinalityListener[V]{}
	}
	ls = append(ls, toAdd)
	c.txIDListeners[txID] = ls
}

func (c *FinalityManager[V]) RemoveListener(txID TxID, toRemove FinalityListener[V]) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(txID) == 0 {
		c.removeAllListener(toRemove)
		return
	}

	ls, ok := c.txIDListeners[txID]
	if !ok {
		return
	}
	for i, l := range ls {
		if l == toRemove {
			ls = append(ls[:i], ls[i+1:]...)
			c.txIDListeners[txID] = ls
			if len(ls) == 0 {
				delete(c.txIDListeners, txID)
			}
			return
		}
	}
}

func (c *FinalityManager[V]) removeAllListener(toRemove FinalityListener[V]) {
	ls := c.allListeners
	for i, l := range ls {
		if l == toRemove {
			c.allListeners = append(ls[:i], ls[i+1:]...)
			return
		}
	}
}

func (c *FinalityManager[V]) Post(event FinalityEvent[V]) {
	c.EventQueue <- event
}

func (c *FinalityManager[V]) Dispatch(event FinalityEvent[V]) {
	l := c.cloneListeners(event.TxID)
	for _, listener := range l {
		c.invokeListener(listener, event.TxID, event.ValidationCode, event.ValidationMessage)
	}
}

func (c *FinalityManager[V]) Run(context context.Context) {
	go c.runEventQueue(context)
	go c.runStatusListener(context)
}

func (c *FinalityManager[V]) invokeListener(l FinalityListener[V], txID TxID, status V, statusMessage string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("caught panic while running dispatching event [%s:%d:%s]: [%s][%s]", txID, status, statusMessage, r, debug.Stack())
		}
	}()
	l.OnStatus(txID, status, statusMessage)
}

func (c *FinalityManager[V]) runEventQueue(context context.Context) {
	for {
		select {
		case <-context.Done():
			return
		case event := <-c.EventQueue:
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
			statuses, err := c.Vault.Statuses(c.txIDs()...)
			if err != nil {
				logger.Errorf("error fetching statuses: %w", err)
				continue
			}
			for _, status := range statuses {
				// check txID status, if it is valid or invalid, post an event
				logger.Debugf("check tx [%s]'s status", status.TxID)
				if c.statuses.Contains(status.ValidationCode) {
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

func (c *FinalityManager[V]) cloneListeners(txID TxID) []FinalityListener[V] {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	ls, ok := c.txIDListeners[txID]
	if !ok {
		return nil
	}
	clone := make([]FinalityListener[V], len(ls))
	copy(clone, ls)
	delete(c.txIDListeners, txID)

	return append(clone, c.allListeners...)
}

func (c *FinalityManager[V]) txIDs() []TxID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return utils.Keys(c.txIDListeners)
}
