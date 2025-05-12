/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package notifier

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

var logger = logging.MustGetLogger()

type pendingOperation struct {
	operation driver.Operation
	payload   map[driver.ColumnKey]string
}

// TODO: More handling is required for cases where we modify twice the same key

type Notifier struct {
	listeners []driver.TriggerCallback
	lMutex    sync.RWMutex

	pending []pendingOperation
	pMutex  sync.RWMutex
}

func NewNotifier() *Notifier {
	return &Notifier{listeners: []driver.TriggerCallback{}}
}

func (db *Notifier) EnqueueEvent(operation driver.Operation, payload map[driver.ColumnKey]string) {
	logger.Warnf("pkey found: %v, operation: %d", payload["pkey"], operation)
	db.pMutex.Lock()
	defer db.pMutex.Unlock()
	db.pending = append(db.pending, pendingOperation{operation: operation, payload: payload})
}

func (db *Notifier) Commit() {
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

func (db *Notifier) Discard() {
	db.pMutex.Lock()
	defer db.pMutex.Unlock()
	db.pending = []pendingOperation{}
}

func (db *Notifier) Subscribe(callback driver.TriggerCallback) error {
	db.lMutex.Lock()
	defer db.lMutex.Unlock()
	db.listeners = append(db.listeners, callback)
	return nil
}

func (db *Notifier) UnsubscribeAll() error {
	db.lMutex.Lock()
	defer db.lMutex.Unlock()
	db.listeners = []driver.TriggerCallback{}
	return nil
}
