/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package notifier

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

// We treat update/inserts as the same, because we don't need the operation type.
// Distinguishing the two cases for sqlite would require more logic.

func NewUnversioned[P driver.UnversionedPersistence](persistence P) *UnversionedPersistenceNotifier[P] {
	return &UnversionedPersistenceNotifier[P]{
		Persistence: persistence,
		Notifier:    NewNotifier(),
	}
}

type UnversionedPersistenceNotifier[P driver.UnversionedPersistence] struct {
	Persistence P
	*Notifier
}

func (db *UnversionedPersistenceNotifier[P]) SetState(ns driver2.Namespace, key driver2.PKey, val driver2.RawValue) error {
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

func (db *UnversionedPersistenceNotifier[P]) SetStates(ns driver2.Namespace, kvs map[driver2.PKey]driver2.RawValue) map[driver2.PKey]error {
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

func (db *UnversionedPersistenceNotifier[P]) Commit() error {
	err := db.Persistence.Commit()
	if err != nil {
		return err
	}
	db.Notifier.Commit()
	return nil
}

func (db *UnversionedPersistenceNotifier[P]) Discard() error {
	err := db.Persistence.Discard()
	if err != nil {
		return err
	}
	db.Notifier.Discard()
	return nil
}

func (db *UnversionedPersistenceNotifier[P]) DeleteState(ns driver2.Namespace, key driver2.PKey) error {
	if err := db.Persistence.DeleteState(ns, key); err != nil {
		return err
	}
	db.Notifier.EnqueueEvent(driver.Delete, map[driver.ColumnKey]string{"ns": ns, "pkey": key})
	return nil
}

func (db *UnversionedPersistenceNotifier[P]) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	errs := db.Persistence.DeleteStates(namespace, keys...)

	for _, key := range keys {
		if _, ok := errs[key]; !ok {
			db.Notifier.EnqueueEvent(driver.Delete, map[driver.ColumnKey]string{"ns": namespace, "pkey": key})
		}
	}

	return errs
}

func (db *UnversionedPersistenceNotifier[P]) GetState(namespace driver2.Namespace, key driver2.PKey) (driver2.RawValue, error) {
	return db.Persistence.GetState(namespace, key)
}

func (db *UnversionedPersistenceNotifier[P]) GetStateRangeScanIterator(namespace driver2.Namespace, startKey, endKey driver2.PKey) (driver.UnversionedResultsIterator, error) {
	return db.Persistence.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (db *UnversionedPersistenceNotifier[P]) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (driver.UnversionedResultsIterator, error) {
	return db.Persistence.GetStateSetIterator(ns, keys...)
}

func (db *UnversionedPersistenceNotifier[P]) Close() error { return db.Persistence.Close() }

func (db *UnversionedPersistenceNotifier[P]) BeginUpdate() error { return db.Persistence.BeginUpdate() }

func (db *UnversionedPersistenceNotifier[P]) Subscribe(callback driver.TriggerCallback) error {
	return db.Notifier.Subscribe(callback)
}

func (db *UnversionedPersistenceNotifier[P]) UnsubscribeAll() error {
	return db.Notifier.UnsubscribeAll()
}

func NewVersioned[P driver.VersionedPersistence](persistence P) *VersionedPersistenceNotifier[P] {
	return &VersionedPersistenceNotifier[P]{
		Persistence: persistence,
		Notifier:    NewNotifier(),
	}
}

type VersionedPersistenceNotifier[P driver.VersionedPersistence] struct {
	Persistence P
	*Notifier
}

func (db *VersionedPersistenceNotifier[P]) SetState(namespace driver2.Namespace, key driver2.PKey, value driver.VersionedValue) error {
	if err := db.Persistence.SetState(namespace, key, value); err != nil {
		return err
	}
	db.Notifier.EnqueueEvent(driver.Update, map[driver.ColumnKey]string{"ns": namespace, "pkey": key})
	return nil
}

func (db *VersionedPersistenceNotifier[P]) SetStates(ns driver2.Namespace, kvs map[driver2.PKey]driver.VersionedValue) map[driver2.PKey]error {
	errs := db.Persistence.SetStates(ns, kvs)

	for key := range kvs {
		if _, ok := errs[key]; !ok {
			db.Notifier.EnqueueEvent(driver.Update, map[driver.ColumnKey]string{"ns": ns, "pkey": key})
		}
	}

	return errs
}

func (db *VersionedPersistenceNotifier[P]) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	errs := db.Persistence.DeleteStates(namespace, keys...)

	for _, key := range keys {
		if _, ok := errs[key]; !ok {
			db.Notifier.EnqueueEvent(driver.Delete, map[driver.ColumnKey]string{"ns": namespace, "pkey": key})
		}
	}

	return errs
}

func (db *VersionedPersistenceNotifier[P]) Commit() error {
	err := db.Persistence.Commit()
	if err != nil {
		return err
	}
	db.Notifier.Commit()
	return nil
}

func (db *VersionedPersistenceNotifier[P]) Discard() error {
	err := db.Persistence.Discard()
	if err != nil {
		return err
	}
	db.Notifier.Discard()
	return nil
}

func (db *VersionedPersistenceNotifier[P]) DeleteState(ns driver2.Namespace, key driver2.PKey) error {
	if err := db.Persistence.DeleteState(ns, key); err != nil {
		return err
	}
	db.Notifier.EnqueueEvent(driver.Delete, map[driver.ColumnKey]string{"ns": ns, "pkey": key})
	return nil
}

func (db *VersionedPersistenceNotifier[P]) GetState(namespace driver2.Namespace, key driver2.PKey) (driver.VersionedValue, error) {
	return db.Persistence.GetState(namespace, key)
}

func (db *VersionedPersistenceNotifier[P]) GetStateMetadata(namespace driver2.Namespace, key driver2.PKey) (driver2.Metadata, driver2.RawVersion, error) {
	return db.Persistence.GetStateMetadata(namespace, key)
}

func (db *VersionedPersistenceNotifier[P]) SetStateMetadata(namespace driver2.Namespace, key driver2.PKey, metadata driver2.Metadata, version driver2.RawVersion) error {
	return db.Persistence.SetStateMetadata(namespace, key, metadata, nil)
}

func (db *VersionedPersistenceNotifier[P]) SetStateMetadatas(ns driver2.Namespace, kvs map[driver2.PKey]driver2.VersionedMetadataValue) map[driver2.PKey]error {
	return db.Persistence.SetStateMetadatas(ns, kvs)
}

func (db *VersionedPersistenceNotifier[P]) GetStateRangeScanIterator(namespace driver2.Namespace, startKey, endKey driver2.PKey) (driver.VersionedResultsIterator, error) {
	return db.Persistence.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (db *VersionedPersistenceNotifier[P]) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (driver.VersionedResultsIterator, error) {
	return db.Persistence.GetStateSetIterator(ns, keys...)
}

func (db *VersionedPersistenceNotifier[P]) Close() error { return db.Persistence.Close() }

func (db *VersionedPersistenceNotifier[P]) BeginUpdate() error { return db.Persistence.BeginUpdate() }

func (db *VersionedPersistenceNotifier[P]) Subscribe(callback driver.TriggerCallback) error {
	return db.Notifier.Subscribe(callback)
}

func (db *VersionedPersistenceNotifier[P]) UnsubscribeAll() error {
	return db.Notifier.UnsubscribeAll()
}
