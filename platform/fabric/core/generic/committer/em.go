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
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

// EventManager manages events for the commit pipeline.
// It consists of a central queue of events.
// The queue is fed by multiple sources.
// A single thread reads from this queue and invokes the listeners in a blocking way
type EventManager struct {
	EventQueue chan TxEvent
	Vault      driver.Vault

	allListeners  []driver.FinalityListener
	txIDListeners map[string][]driver.FinalityListener
	mutex         sync.RWMutex
}

func NewEventManager(vault driver.Vault, size int) *EventManager {
	return &EventManager{
		EventQueue:    make(chan TxEvent, size),
		Vault:         vault,
		allListeners:  nil,
		txIDListeners: map[string][]driver.FinalityListener{},
	}
}

func (c *EventManager) AddListener(txID string, toAdd driver.FinalityListener) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(txID) == 0 {
		c.allListeners = append(c.allListeners, toAdd)
	}

	ls, ok := c.txIDListeners[txID]
	if !ok {
		ls = []driver.FinalityListener{}
		c.txIDListeners[txID] = ls
	}
	ls = append(ls, toAdd)
	c.txIDListeners[txID] = ls
}

func (c *EventManager) RemoveListener(txID string, toRemove driver.FinalityListener) {
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

func (c *EventManager) removeAllListener(toRemove driver.FinalityListener) {
	ls := c.allListeners
	for i, l := range ls {
		if l == toRemove {
			c.allListeners = append(ls[:i], ls[i+1:]...)
			return
		}
	}
}

func (c *EventManager) Post(event TxEvent) {
	c.EventQueue <- event
}

func (c *EventManager) Dispatch(event TxEvent) {
	l := c.cloneListeners(event.TxID)
	for _, listener := range l {
		c.invokeListener(listener, event.TxID, int(event.ValidationCode), event.ValidationMessage)
	}
}

func (c *EventManager) Run(context context.Context) {
	go c.runEventQueue(context)
	go c.runStatusListener(context)
}

func (c *EventManager) invokeListener(l driver.FinalityListener, txID string, status int, statusMessage string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("caught panic while running dispatching event [%s:%d:%s]: [%s][%s]", txID, status, statusMessage, r, debug.Stack())
		}
	}()
	l.OnStatus(txID, status, statusMessage)
}

func (c *EventManager) runEventQueue(context context.Context) {
	for {
		select {
		case <-context.Done():
			return
		case event := <-c.EventQueue:
			c.Dispatch(event)
		}
	}
}

func (c *EventManager) runStatusListener(context context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-context.Done():
			return
		case <-ticker.C:
			txIDs := c.txIDs()
			for _, txID := range txIDs {
				// check txID status, if it is valid or invalid, post an event
				logger.Debugf("check tx [%s]'s status", txID)
				vc, message, err := c.Vault.Status(txID)
				if err == nil && (vc == driver.Valid || vc == driver.Invalid) {
					// post the event
					c.Post(TxEvent{
						TxID:              txID,
						ValidationCode:    pb.TxValidationCode(vc),
						ValidationMessage: message,
					})
				}
			}
		}
	}
}

func (c *EventManager) cloneListeners(txID string) []driver.FinalityListener {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	ls, ok := c.txIDListeners[txID]
	if !ok {
		return nil
	}
	clone := make([]driver.FinalityListener, len(ls))
	copy(clone, ls)
	delete(c.txIDListeners, txID)

	return append(clone, c.allListeners...)
}

func (c *EventManager) txIDs() []string {
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
