/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/test-go/testify/assert"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
)

func TestTXIDStoreMem(t *testing.T) {
	db, err := db.Open("memory", "")
	assert.NoError(t, err)
	assert.NotNil(t, db)
	store, err := NewTXIDStore(db)
	assert.NoError(t, err)
	assert.NotNil(t, store)

	testTXIDStore(t, store)

	store, err = NewTXIDStore(db)
	assert.NoError(t, err)
	assert.NotNil(t, store)

	testOneMore(t, store)
}

func TestTXIDStoreBadger(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "TestTXIDStoreBadger")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	db, err := db.Open("badger", tempDir)
	assert.NoError(t, err)
	assert.NotNil(t, db)
	store, err := NewTXIDStore(db)
	assert.NoError(t, err)
	assert.NotNil(t, store)

	testTXIDStore(t, store)

	store, err = NewTXIDStore(db)
	assert.NoError(t, err)
	assert.NotNil(t, store)

	testOneMore(t, store)
}

func testOneMore(t *testing.T, store *TXIDStore) {
	err := store.persistence.BeginUpdate()
	assert.NoError(t, err)
	err = store.Set("txid3", driver.Valid)
	assert.NoError(t, err)
	err = store.persistence.Commit()
	assert.NoError(t, err)

	status, err := store.Get("txid3")
	assert.NoError(t, err)
	assert.Equal(t, driver.Valid, status)

	it, err := store.Iterator(&driver.SeekStart{})
	assert.NoError(t, err)
	txids := []string{}
	for {
		tid, err := it.Next()
		assert.NoError(t, err)

		if tid == nil {
			it.Close()
			break
		}

		txids = append(txids, tid.Txid)
	}
	assert.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3"}, txids)

	it, err = store.Iterator(&driver.SeekEnd{})
	assert.NoError(t, err)
	txids = []string{}
	for {
		tid, err := it.Next()
		assert.NoError(t, err)

		if tid == nil {
			it.Close()
			break
		}

		txids = append(txids, tid.Txid)
	}
	assert.Equal(t, []string{"txid3"}, txids)

	last, err := store.GetLastTxID()
	assert.NoError(t, err)
	assert.Equal(t, "txid3", last)
}

func testTXIDStore(t *testing.T, store *TXIDStore) {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("%s", r)
			}
		}()

		store.Set("txid1", driver.Valid)
	}()
	assert.EqualError(t, err, "programming error, writing without ongoing update")

	it, err := store.Iterator(&driver.SeekEnd{})
	assert.NoError(t, err)
	next, err := it.Next()
	assert.NoError(t, err)
	assert.Nil(t, next)

	err = store.persistence.BeginUpdate()
	assert.NoError(t, err)
	err = store.Set("txid1", driver.Valid)
	assert.NoError(t, err)
	err = store.Set("txid2", driver.Valid)
	assert.NoError(t, err)
	err = store.Set("txid10", driver.Valid)
	assert.NoError(t, err)
	err = store.Set("txid12", driver.Valid)
	assert.NoError(t, err)
	err = store.Set("txid21", driver.Valid)
	assert.NoError(t, err)
	err = store.Set("txid100", driver.Valid)
	assert.NoError(t, err)
	err = store.Set("txid200", driver.Valid)
	assert.NoError(t, err)
	err = store.Set("txid1025", driver.Valid)
	assert.NoError(t, err)
	err = store.persistence.Commit()
	assert.NoError(t, err)

	status, err := store.Get("txid3")
	assert.NoError(t, err)
	assert.Equal(t, driver.Unknown, status)
	status, err = store.Get("txid10")
	assert.NoError(t, err)
	assert.Equal(t, driver.Valid, status)

	_, err = store.Iterator(&struct{}{})
	assert.EqualError(t, err, "invalid position *struct {}")

	it, err = store.Iterator(&driver.SeekEnd{})
	assert.NoError(t, err)
	txids := []string{}
	for {
		tid, err := it.Next()
		assert.NoError(t, err)

		if tid == nil {
			it.Close()
			break
		}

		txids = append(txids, tid.Txid)
	}
	assert.Equal(t, []string{"txid1025"}, txids)

	it, err = store.Iterator(&driver.SeekStart{})
	assert.NoError(t, err)
	txids = []string{}
	for {
		tid, err := it.Next()
		assert.NoError(t, err)

		if tid == nil {
			it.Close()
			break
		}

		txids = append(txids, tid.Txid)
	}
	assert.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025"}, txids)

	it, err = store.Iterator(&driver.SeekPos{Txid: "boh"})
	assert.EqualError(t, err, "txid boh was not found")

	it, err = store.Iterator(&driver.SeekPos{Txid: "txid12"})
	assert.NoError(t, err)
	txids = []string{}
	for {
		tid, err := it.Next()
		assert.NoError(t, err)

		if tid == nil {
			it.Close()
			break
		}

		txids = append(txids, tid.Txid)
	}
	assert.Equal(t, []string{"txid12", "txid21", "txid100", "txid200", "txid1025"}, txids)
}
