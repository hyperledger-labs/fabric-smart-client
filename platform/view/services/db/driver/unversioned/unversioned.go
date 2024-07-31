/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package unversioned

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
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

func (db *Unversioned) SetState(namespace, key string, value []byte) error {
	return db.Versioned.SetState(namespace, key, driver.VersionedValue{Raw: value})
}

func (db *Unversioned) GetState(namespace, key string) ([]byte, error) {
	vv, err := db.Versioned.GetState(namespace, key)
	return vv.Raw, err
}

func (db *Unversioned) DeleteState(namespace, key string) error {
	return db.Versioned.DeleteState(namespace, key)
}

func (db *Unversioned) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.UnversionedResultsIterator, error) {
	vitr, err := db.Versioned.GetStateRangeScanIterator(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}

	return &iterator{vitr}, nil
}

func (db *Unversioned) GetStateSetIterator(ns string, keys ...string) (driver.UnversionedResultsIterator, error) {
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

type Transactional struct {
	TransactionalVersioned driver.TransactionalVersionedPersistence
}

func (t *Transactional) SetState(namespace driver2.Namespace, key string, value []byte) error {
	return t.TransactionalVersioned.SetState(namespace, key, driver.VersionedValue{Raw: value})
}

func (t *Transactional) GetState(namespace driver2.Namespace, key string) ([]byte, error) {
	read, err := t.TransactionalVersioned.GetState(namespace, key)
	if err != nil {
		return nil, err
	}
	return read.Raw, nil
}

func (t *Transactional) DeleteState(namespace driver2.Namespace, key string) error {
	return t.TransactionalVersioned.DeleteState(namespace, key)
}

func (t *Transactional) GetStateRangeScanIterator(namespace driver2.Namespace, startKey string, endKey string) (driver.UnversionedResultsIterator, error) {
	it, err := t.TransactionalVersioned.GetStateRangeScanIterator(namespace, startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &iterator{itr: it}, nil
}

func (t *Transactional) GetStateSetIterator(ns driver2.Namespace, keys ...string) (driver.UnversionedResultsIterator, error) {
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
		return nil, err
	}
	return &WriteTransaction{WriteTransaction: tx}, nil
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
