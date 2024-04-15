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

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// FinalityEvent contains information about the finality of a given transaction
type FinalityEvent struct {
	TxID              string
	ValidationCode    int
	ValidationMessage string
	Block             uint64
	IndexInBlock      int
	Err               error
}

// FinalityListener is the interface that must be implemented to receive transaction status notifications
type FinalityListener interface {
	// OnStatus is called when the status of a transaction changes, or it is valid or invalid
	OnStatus(txID string, status int, statusMessage string)
}

type Vault interface {
	Statuses(txIDs ...string) ([]driver.TxValidationStatus, error)
}

// FinalityManager manages events for the commit pipeline.
// It consists of a central queue of events.
// The queue is fed by multiple sources.
// A single thread reads from this queue and invokes the listeners in a blocking way
type FinalityManager struct {
	EventQueue chan FinalityEvent
	Vault      Vault

	statuses      []int
	allListeners  []FinalityListener
	txIDListeners map[string][]FinalityListener
	mutex         sync.RWMutex
}

func NewFinalityManager(vault Vault, size int, statuses []int) *FinalityManager {
	return &FinalityManager{
		EventQueue:    make(chan FinalityEvent, size),
		Vault:         vault,
		statuses:      statuses,
		allListeners:  nil,
		txIDListeners: map[string][]FinalityListener{},
	}
}

func (c *FinalityManager) AddListener(txID string, toAdd FinalityListener) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(txID) == 0 {
		c.allListeners = append(c.allListeners, toAdd)
	}

	ls, ok := c.txIDListeners[txID]
	if !ok {
		ls = []FinalityListener{}
		c.txIDListeners[txID] = ls
	}
	ls = append(ls, toAdd)
	c.txIDListeners[txID] = ls
}

func (c *FinalityManager) RemoveListener(txID string, toRemove FinalityListener) {
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

func (c *FinalityManager) removeAllListener(toRemove FinalityListener) {
	ls := c.allListeners
	for i, l := range ls {
		if l == toRemove {
			c.allListeners = append(ls[:i], ls[i+1:]...)
			return
		}
	}
}

func (c *FinalityManager) Post(event FinalityEvent) {
	c.EventQueue <- event
}

func (c *FinalityManager) Dispatch(event FinalityEvent) {
	l := c.cloneListeners(event.TxID)
	for _, listener := range l {
		c.invokeListener(listener, event.TxID, event.ValidationCode, event.ValidationMessage)
	}
}

func (c *FinalityManager) Run(context context.Context) {
	go c.runEventQueue(context)
	go c.runStatusListener(context)
}

func (c *FinalityManager) invokeListener(l FinalityListener, txID string, status int, statusMessage string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("caught panic while running dispatching event [%s:%d:%s]: [%s][%s]", txID, status, statusMessage, r, debug.Stack())
		}
	}()
	l.OnStatus(txID, status, statusMessage)
}

func (c *FinalityManager) runEventQueue(context context.Context) {
	for {
		select {
		case <-context.Done():
			return
		case event := <-c.EventQueue:
			c.Dispatch(event)
		}
	}
}

func (c *FinalityManager) runStatusListener(context context.Context) {
	ticker := time.NewTicker(1 * time.Second)
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
				for _, target := range c.statuses {
					if int(status.ValidationCode) == target {
						// post the event
						c.Post(FinalityEvent{
							TxID:              status.TxID,
							ValidationCode:    int(status.ValidationCode),
							ValidationMessage: status.Message,
						})
						break
					}
				}
			}
		}
	}
}

func (c *FinalityManager) cloneListeners(txID string) []FinalityListener {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	ls, ok := c.txIDListeners[txID]
	if !ok {
		return nil
	}
	clone := make([]FinalityListener, len(ls))
	copy(clone, ls)
	delete(c.txIDListeners, txID)

	return append(clone, c.allListeners...)
}

func (c *FinalityManager) txIDs() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	res := make([]string, len(c.txIDListeners))
	i := 0
	for txID := range c.txIDListeners {
		res[i] = txID
		i++
	}

	return res
}
