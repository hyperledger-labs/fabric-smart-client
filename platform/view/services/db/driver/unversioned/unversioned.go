/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package unversioned

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

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
