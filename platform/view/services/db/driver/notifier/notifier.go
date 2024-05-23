/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package notifier

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("postgres-db")

type pendingOperation struct {
	operation driver.Operation
	payload   map[driver.ColumnKey]string
}

// TODO: More handling is required for cases where we modify twice the same key

type notifier struct {
	listeners []driver.TriggerCallback
	lMutex    sync.RWMutex

	pending []pendingOperation
	pMutex  sync.RWMutex
}

func newNotifier() *notifier {
	return &notifier{listeners: []driver.TriggerCallback{}}
}

func (db *notifier) enqueueEvent(operation driver.Operation, payload map[driver.ColumnKey]string) {
	logger.Warnf("pkey found: %v, operation: %d", payload["pkey"], operation)
	db.pMutex.Lock()
	defer db.pMutex.Unlock()
	db.pending = append(db.pending, pendingOperation{operation: operation, payload: payload})
}

func (db *notifier) commit() {
	db.pMutex.Lock()
	defer db.pMutex.Unlock()
	db.lMutex.RLock()
	defer db.lMutex.RUnlock()
	for _, op := range db.pending {
		for _, listener := range db.listeners {
			listener(op.operation, op.payload)
		}
	}
	db.pending = []pendingOperation{}
}

func (db *notifier) discard() {
	db.pMutex.Lock()
	defer db.pMutex.Unlock()
	db.pending = []pendingOperation{}
}

func (db *notifier) subscribe(callback driver.TriggerCallback) error {
	db.lMutex.Lock()
	defer db.lMutex.Unlock()
	db.listeners = append(db.listeners, callback)
	return nil
}

func (db *notifier) unsubscribeAll() error {
	db.lMutex.Lock()
	defer db.lMutex.Unlock()
	db.listeners = []driver.TriggerCallback{}
	return nil
}
