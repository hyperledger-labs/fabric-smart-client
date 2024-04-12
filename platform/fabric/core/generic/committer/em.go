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

// EventManager manages events for the commit pipeline.
// It consists of a central queue of events.
// The queue is fed by multiple sources.
// A single thread reads from this queue and invokes the listeners in a blocking way
type EventManager struct {
	EventQueue chan TxEvent

	allListeners  []driver.TxStatusListener
	txIDListeners map[string][]driver.TxStatusListener
	mutex         sync.RWMutex
}

func NewEventManager(size int) *EventManager {
	return &EventManager{
		EventQueue:    make(chan TxEvent, size),
		allListeners:  nil,
		txIDListeners: map[string][]driver.TxStatusListener{},
	}
}

func (c *EventManager) AddListener(txID string, l driver.TxStatusListener) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(txID) == 0 {
		c.allListeners = append(c.allListeners, l)
	}

	ls, ok := c.txIDListeners[txID]
	if !ok {
		ls = []driver.TxStatusListener{}
		c.txIDListeners[txID] = ls
	}
	ls = append(ls, l)
	c.txIDListeners[txID] = ls
}

func (c *EventManager) DeleteListener(txID string, ch driver.TxStatusListener) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	var ls []driver.TxStatusListener
	if len(txID) == 0 {
		ls = c.allListeners
	} else {
		var ok bool
		ls, ok = c.txIDListeners[txID]
		if !ok {
			return
		}
	}
	for i, l := range ls {
		if l == ch {
			ls = append(ls[:i], ls[i+1:]...)
			c.txIDListeners[txID] = ls
			return
		}
	}
}

func (c *EventManager) Post(event TxEvent) {
	c.EventQueue <- event
}

func (c *EventManager) Dispatch(event TxEvent) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("caught panic while running dispatching event [%v]: [%s][%s]", event, r, debug.Stack())
		}
	}()
	l := c.cloneListeners(event.TxID)
	for _, listener := range l {
		if err := listener.OnStatus(event.TxID, int(event.ValidationCode), event.ValidationMessage); err != nil {
			logger.Errorf("failed on status call [%v]: [%s][%s]", event, err, debug.Stack())
		}
		c.DeleteListener(event.TxID, listener)
	}
}

func (c *EventManager) Run(context context.Context) {
	go c.runEventQueue(context)
	go c.runStatusListener(context)
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
			}
		}
	}
}

func (c *EventManager) cloneListeners(txID string) []driver.TxStatusListener {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	ls, ok := c.txIDListeners[txID]
	if !ok {
		return nil
	}

	clone := make([]driver.TxStatusListener, len(ls))
	copy(clone, ls)
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
