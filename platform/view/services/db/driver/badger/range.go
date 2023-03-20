/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"bytes"
	"strings"

	"github.com/dgraph-io/badger/v3"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/pkg/errors"
)

var iteratorOptions = badger.IteratorOptions{
	PrefetchValues: false,
	PrefetchSize:   100,
	Reverse:        false,
	AllVersions:    false,
}

type rangeScanIterator struct {
	txn       *badger.Txn
	it        *badger.Iterator
	startKey  string
	endKey    string
	namespace string
}

func (r *rangeScanIterator) Next() (*driver.VersionedRead, error) {
	if !r.it.Valid() {
		return nil, nil
	}

	item := r.it.Item()
	if r.endKey != "" && (bytes.Compare(item.Key(), []byte(dbKey(r.namespace, r.endKey))) >= 0) {
		return nil, nil
	}

	v, err := versionedValue(item, string(item.Key()))
	if err != nil {
		return nil, errors.Wrapf(err, "error iterating on range %s:%s", r.startKey, r.endKey)
	}

	dbKey := string(item.Key())
	dbKey = dbKey[strings.Index(dbKey, keys.NamespaceSeparator)+1:]

	r.it.Next()

	return &driver.VersionedRead{
		Key:          dbKey,
		Block:        v.Block,
		IndexInBlock: int(v.Txnum),
		Raw:          v.Value,
	}, nil
}

func (r *rangeScanIterator) Close() {
	r.it.Close()
	r.txn.Discard()
}

func (db *DB) GetStateRangeScanIterator(namespace string, startKey string, endKey string) (driver.VersionedResultsIterator, error) {
	txn := db.db.NewTransaction(false)
	it := txn.NewIterator(iteratorOptions)
	it.Seek([]byte(dbKey(namespace, startKey)))

	return &rangeScanIterator{
		txn:       txn,
		it:        it,
		startKey:  startKey,
		endKey:    endKey,
		namespace: namespace,
	}, nil
}
