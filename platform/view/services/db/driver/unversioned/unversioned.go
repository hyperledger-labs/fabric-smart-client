/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package unversioned

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
)

type iterator struct {
	itr driver.VersionedResultsIterator
}

func (i *iterator) Next() (*driver.UnversionedRead, error) {
	r, err := i.itr.Next()
	if err != nil {
		return nil, err
	}

	if r == nil {
		return nil, nil
	}

	return &driver.UnversionedRead{
		Key: r.Key,
		Raw: r.Raw,
	}, nil
}

func (i *iterator) Close() {
	i.itr.Close()
}

type Unversioned struct {
	Versioned driver.VersionedPersistence
}

func (db *Unversioned) SetState(namespace driver2.Namespace, key driver2.PKey, value driver2.RawValue) error {
	return db.Versioned.SetState(namespace, key, driver.VersionedValue{Raw: value})
}

func (db *Unversioned) SetStates(namespace driver2.Namespace, kvs map[driver2.PKey]driver2.RawValue) map[driver2.PKey]error {
	versioned := make(map[driver2.PKey]driver.VersionedValue, len(kvs))
	for k, v := range kvs {
		versioned[k] = driver.VersionedValue{Raw: v}
	}
	return db.Versioned.SetStates(namespace, versioned)
}

func (db *Unversioned) GetState(namespace driver2.Namespace, key driver2.PKey) (driver2.RawValue, error) {
	vv, err := db.Versioned.GetState(namespace, key)
	return vv.Raw, err
}

func (db *Unversioned) DeleteState(namespace driver2.Namespace, key driver2.PKey) error {
	return db.Versioned.DeleteState(namespace, key)
}

func (db *Unversioned) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return db.Versioned.DeleteStates(namespace, keys...)
}

func (db *Unversioned) GetStateRangeScanIterator(namespace driver2.Namespace, startKey, endKey driver2.PKey) (driver.UnversionedResultsIterator, error) {
	vitr, err := db.Versioned.GetStateRangeScanIterator(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}

	return &iterator{vitr}, nil
}

func (db *Unversioned) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (driver.UnversionedResultsIterator, error) {
	vitr, err := db.Versioned.GetStateSetIterator(ns, keys...)
	if err != nil {
		return nil, err
	}

	return &iterator{vitr}, nil
}

func (db *Unversioned) Close() error {
	return db.Versioned.Close()
}

func (db *Unversioned) BeginUpdate() error {
	return db.Versioned.BeginUpdate()
}

func (db *Unversioned) Commit() error {
	return db.Versioned.Commit()
}

func (db *Unversioned) Discard() error {
	return db.Versioned.Discard()
}

func (db *Unversioned) Stats() any {
	return db.Versioned.Stats()
}

type Transactional struct {
	TransactionalVersioned driver.TransactionalVersionedPersistence
}

func (t *Transactional) SetState(namespace driver2.Namespace, key driver2.PKey, value driver2.RawValue) error {
	return t.TransactionalVersioned.SetState(namespace, key, driver.VersionedValue{Raw: value})
}

func (t *Transactional) SetStates(namespace driver2.Namespace, kvs map[driver2.PKey]driver2.RawValue) map[driver2.PKey]error {
	versioned := make(map[driver2.PKey]driver.VersionedValue, len(kvs))
	for k, v := range kvs {
		versioned[k] = driver.VersionedValue{Raw: v}
	}
	return t.TransactionalVersioned.SetStates(namespace, versioned)
}

func (t *Transactional) GetState(namespace driver2.Namespace, key driver2.PKey) (driver2.RawValue, error) {
	read, err := t.TransactionalVersioned.GetState(namespace, key)
	if err != nil {
		return nil, err
	}
	return read.Raw, nil
}

func (t *Transactional) DeleteState(namespace driver2.Namespace, key driver2.PKey) error {
	return t.TransactionalVersioned.DeleteState(namespace, key)
}

func (t *Transactional) DeleteStates(namespace driver2.Namespace, keys ...driver2.PKey) map[driver2.PKey]error {
	return t.TransactionalVersioned.DeleteStates(namespace, keys...)
}

func (t *Transactional) GetStateRangeScanIterator(namespace driver2.Namespace, startKey driver2.PKey, endKey driver2.PKey) (driver.UnversionedResultsIterator, error) {
	it, err := t.TransactionalVersioned.GetStateRangeScanIterator(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &iterator{itr: it}, nil
}

func (t *Transactional) GetStateSetIterator(ns driver2.Namespace, keys ...driver2.PKey) (driver.UnversionedResultsIterator, error) {
	it, err := t.TransactionalVersioned.GetStateSetIterator(ns, keys...)
	if err != nil {
		return nil, err
	}
	return &iterator{itr: it}, nil
}

func (t *Transactional) Close() error {
	return t.TransactionalVersioned.Close()
}

func (t *Transactional) BeginUpdate() error {
	return t.TransactionalVersioned.BeginUpdate()
}

func (t *Transactional) Commit() error {
	return t.TransactionalVersioned.Commit()
}

func (t *Transactional) Discard() error {
	return t.TransactionalVersioned.Discard()
}

func (t *Transactional) NewWriteTransaction() (driver.UnversionedWriteTransaction, error) {
	tx, err := t.TransactionalVersioned.NewWriteTransaction()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create new transaction")
	}
	return &WriteTransaction{WriteTransaction: tx}, nil
}

func (t *Transactional) Stats() any {
	return t.TransactionalVersioned.Stats()
}

type WriteTransaction struct {
	WriteTransaction driver.WriteTransaction
}

func (w *WriteTransaction) SetState(namespace driver2.Namespace, key string, value []byte) error {
	return w.WriteTransaction.SetState(namespace, key, driver.VersionedValue{Raw: value})
}

func (w *WriteTransaction) DeleteState(namespace driver2.Namespace, key string) error {
	return w.WriteTransaction.DeleteState(namespace, key)
}

func (w *WriteTransaction) Commit() error {
	return w.WriteTransaction.Commit()
}

func (w *WriteTransaction) Discard() error {
	return w.WriteTransaction.Discard()
}
