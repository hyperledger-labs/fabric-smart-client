/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package simple

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

type eventHandler struct {
	receiver events.Listener
}

type eventBus struct {
	handlers map[string][]*eventHandler
	lock     sync.RWMutex
}

func NewEventBus() *eventBus {
	return &eventBus{
		handlers: make(map[string][]*eventHandler),
		lock:     sync.RWMutex{},
	}
}

func (e *eventBus) Publish(event events.Event) {
	if event == nil {
		return
	}

	e.lock.RLock()
	defer e.lock.RUnlock()

	subs, ok := e.handlers[event.Topic()]
	if !ok {
		// no subscriber ok
		return
	}

	// call all receivers
	for _, sub := range subs {
		sub.receiver.OnReceive(event)
	}
}

func (e *eventBus) Subscribe(topic string, receiver events.Listener) {
	if receiver == nil {
		return
	}

	e.lock.Lock()
	defer e.lock.Unlock()

	handlers := e.handlers[topic]
	e.handlers[topic] = append(handlers, &eventHandler{receiver: receiver})

	return
}

func (e *eventBus) Unsubscribe(topic string, receiver events.Listener) {
	if receiver == nil {
		return
	}

	e.lock.Lock()
	defer e.lock.Unlock()

	handlers, ok := e.handlers[topic]
	if !ok {
		// no subscriber for this topic
		return
	}

	// find receiver
	idx := findIndex(handlers, receiver)
	if idx == -1 {
		// receiver not in list
		return
	}

	// remove receiver at position idx
	last := len(handlers) - 1
	handlers[idx] = handlers[last]
	handlers[last] = nil
	handlers = handlers[0:last]

	if len(handlers) > 0 {
		e.handlers[topic] = handlers
	} else {
		// let's remove topic entry
		delete(e.handlers, topic)
	}
	return
}

// findIndex returns the position of receiver in handlers.
// Returns -1 if not found
func findIndex(handlers []*eventHandler, receiver events.Listener) int {
	for i, h := range handlers {
		if h.receiver == receiver {
			return i
		}
	}

	return -1
}
