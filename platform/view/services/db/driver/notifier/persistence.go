/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package notifier

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

// We treat update/inserts as the same, because we don't need the operation type.
// Distinguishing the two cases for sqlite would require more logic.

func NewUnversioned(persistence driver.KeyValueStore) *UnversionedPersistenceNotifier {
	return &UnversionedPersistenceNotifier{
		Persistence: persistence,
		Notifier:    NewNotifier(),
	}
}

type UnversionedPersistenceNotifier struct {
	Persistence driver.KeyValueStore
	*Notifier
}

func (db *UnversionedPersistenceNotifier) SetState(ns driver2.Namespace, key driver2.PKey, val driver2.RawValue) error {
	if err := db.Persistence.SetState(ns, key, val); err != nil {
		return err
	}
	op := driver.Update
	if len(val) == 0 {
		op = driver.Delete
	}
	db.Notifier.EnqueueEvent(op, map[driver.ColumnKey]string{"ns": ns, "pkey": key})
	return nil
}

func (db *UnversionedPersistenceNotifier) SetStates(ns driver2.Namespace, kvs map[driver2.PKey]driver2.RawValue) map[driver2.PKey]error {
	errs := db.Persistence.SetStates(ns, kvs)

	for key, val := range kvs {
		if _, ok := errs[key]; !ok {
			op := driver.Update
			if len(val) == 0 {
				op = driver.Delete
			}
			db.Notifier.EnqueueEvent(op, map[driver.ColumnKey]string{"ns": ns, "pkey": key})
		}
	}

	return errs
}

func (db *UnversionedPersistenceNotifier) Commit() error {
	err := db.Persistence.Commit()
	if err != nil {
		return err
	}
	db.Notifier.Commit()
	return nil
}

func (db *UnversionedPersistenceNotifier) Discard() error {
	err := db.Persistence.Discard()
	if err != nil {
		return err
	}
	db.Notifier.Discard()
	return nil
}

func (db *UnversionedPersistenceNotifier) DeleteState(ns driver2.Namespace, key driver2.PKey) error {
	if err := db.Persistence.DeleteState(ns, key); err != nil {
		return err
	}
	db.Notifier.EnqueueEvent(driver.Delete, map[driver.ColumnKey]string{"ns": ns, "pkey": key})
	return nil
}

func (db *UnversionedPersistenceNotifier) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	errs := db.Persistence.DeleteStates(namespace, keys...)

	for _, key := range keys {
		if _, ok := errs[key]; !ok {
			db.Notifier.EnqueueEvent(driver.Delete, map[driver.ColumnKey]string{"ns": namespace, "pkey": key})
		}
	}

	return errs
}

func (db *UnversionedPersistenceNotifier) GetState(namespace driver2.Namespace, key driver2.PKey) (driver2.RawValue, error) {
	return db.Persistence.GetState(namespace, key)
}

func (db *UnversionedPersistenceNotifier) GetStateRangeScanIterator(namespace driver2.Namespace, startKey, endKey driver2.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	return db.Persistence.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (db *UnversionedPersistenceNotifier) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (collections.Iterator[*driver.UnversionedRead], error) {
	return db.Persistence.GetStateSetIterator(ns, keys...)
}

func (db *UnversionedPersistenceNotifier) Close() error { return db.Persistence.Close() }

func (db *UnversionedPersistenceNotifier) BeginUpdate() error { return db.Persistence.BeginUpdate() }

func (db *UnversionedPersistenceNotifier) Stats() any {
	return db.Persistence.Stats()
}

func (db *UnversionedPersistenceNotifier) Subscribe(callback driver.TriggerCallback) error {
	return db.Notifier.Subscribe(callback)
}

func (db *UnversionedPersistenceNotifier) UnsubscribeAll() error {
	return db.Notifier.UnsubscribeAll()
}
