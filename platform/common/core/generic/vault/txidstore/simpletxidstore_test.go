/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/pkg/errors"
	"github.com/test-go/testify/assert"
	"golang.org/x/exp/slices"
)

type vc int

const (
	_ vc = iota
	valid
	invalid
	busy
	unknown
)

type vcProvider struct{}

func (p *vcProvider) ToInt32(code vc) int32 { return int32(code) }
func (p *vcProvider) FromInt32(code int32) vc {
	return vc(code)
}
func (p *vcProvider) Unknown() vc  { return unknown }
func (p *vcProvider) Busy() vc     { return busy }
func (p *vcProvider) Valid() vc    { return valid }
func (p *vcProvider) Invalid() vc  { return invalid }
func (p *vcProvider) NotFound() vc { return 0 }

// When we ask for a TX that does not exist in the DB, some DBs return the key with a nil value and others don't return it at all.
var removeNils func(items []*driver.ByNum[vc]) []*driver.ByNum[vc]

func TestTXIDStoreMem(t *testing.T) {
	removeNils = func(items []*driver.ByNum[vc]) []*driver.ByNum[vc] {
		return slices.DeleteFunc(items, func(e *driver.ByNum[vc]) bool { return e.Code == 0 })
	}
	db, err := db.OpenMemory()
	assert.NoError(t, err)

	testAll(t, db)
}

func TestTXIDStoreBadger(t *testing.T) {
	removeNils = func(items []*driver.ByNum[vc]) []*driver.ByNum[vc] {
		return slices.DeleteFunc(items, func(e *driver.ByNum[vc]) bool { return e.Code == 0 })
	}
	db, err := db.OpenBadger(t.TempDir(), "TestTXIDStoreBadger")
	assert.NoError(t, err)

	testAll(t, db)
}

func TestTXIDStoreSqlite(t *testing.T) {
	removeNils = func(items []*driver.ByNum[vc]) []*driver.ByNum[vc] { return items }
	db, err := db.OpenSqlite("testdb", t.TempDir())
	assert.NoError(t, err)
	defer db.Close()

	testAll(t, db)
}

func TestTXIDStorePostgres(t *testing.T) {
	removeNils = func(items []*driver.ByNum[vc]) []*driver.ByNum[vc] { return items }
	db, terminate, err := db.OpenPostgres("testdb")
	assert.NoError(t, err)
	defer db.Close()
	defer terminate()

	testAll(t, db)
}

func testAll(t *testing.T, db UnversionedPersistence) {
	assert.NotNil(t, db)
	store, err := NewSimpleTXIDStore[vc](db, &vcProvider{})
	assert.NoError(t, err)
	assert.NotNil(t, store)

	testTXIDStore(t, store)

	store, err = NewSimpleTXIDStore[vc](db, &vcProvider{})
	assert.NoError(t, err)
	assert.NotNil(t, store)

	testOneMore(t, store)
}

func testOneMore(t *testing.T, store *SimpleTXIDStore[vc]) {
	err := store.Persistence.BeginUpdate()
	assert.NoError(t, err)
	err = store.Set("txid3", valid, "")
	assert.NoError(t, err)
	err = store.Persistence.Commit()
	assert.NoError(t, err)

	status, _, err := store.Get("txid3")
	assert.NoError(t, err)
	assert.Equal(t, valid, status)

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

		txids = append(txids, tid.TxID)
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

		txids = append(txids, tid.TxID)
	}
	assert.Equal(t, []string{"txid3"}, txids)

	last, err := store.GetLastTxID()
	assert.NoError(t, err)
	assert.Equal(t, "txid3", last)

	// add a busy tx
	err = store.Persistence.BeginUpdate()
	assert.NoError(t, err)
	err = store.Set("txid4", busy, "")
	assert.NoError(t, err)
	err = store.Persistence.Commit()
	assert.NoError(t, err)

	last, err = store.GetLastTxID()
	assert.NoError(t, err)
	assert.Equal(t, "txid3", last)

	// iterate again
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

		txids = append(txids, tid.TxID)
	}
	assert.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3", "txid4"}, txids)

	// update the busy tx
	err = store.Persistence.BeginUpdate()
	assert.NoError(t, err)
	err = store.Set("txid4", valid, "")
	assert.NoError(t, err)
	err = store.Persistence.Commit()
	assert.NoError(t, err)

	last, err = store.GetLastTxID()
	assert.NoError(t, err)
	assert.Equal(t, "txid4", last)

	// iterate again
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

		txids = append(txids, tid.TxID)
	}
	assert.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3", "txid4", "txid4"}, txids)
}

func testTXIDStore(t *testing.T, store *SimpleTXIDStore[vc]) {
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = errors.Errorf("%s", r)
			}
		}()

		store.Set("txid1", valid, "")
	}()
	assert.EqualError(t, err, "programming error, writing without ongoing update")

	it, err := store.Iterator(&driver.SeekEnd{})
	assert.NoError(t, err)
	next, err := it.Next()
	assert.NoError(t, err)
	assert.Nil(t, next)

	err = store.Persistence.BeginUpdate()
	assert.NoError(t, err)
	err = store.Set("txid1", valid, "")
	assert.NoError(t, err)
	err = store.Set("txid2", valid, "")
	assert.NoError(t, err)
	err = store.Set("txid10", valid, "")
	assert.NoError(t, err)
	err = store.Set("txid12", valid, "")
	assert.NoError(t, err)
	err = store.Set("txid21", valid, "")
	assert.NoError(t, err)
	err = store.Set("txid100", valid, "")
	assert.NoError(t, err)
	err = store.Set("txid200", valid, "")
	assert.NoError(t, err)
	err = store.Set("txid1025", invalid, "")
	assert.NoError(t, err)
	err = store.Persistence.Commit()
	assert.NoError(t, err)

	status, _, err := store.Get("txid3")
	assert.NoError(t, err)
	assert.Equal(t, unknown, status)
	status, _, err = store.Get("txid10")
	assert.NoError(t, err)
	assert.Equal(t, valid, status)

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

		txids = append(txids, tid.TxID)
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

		txids = append(txids, tid.TxID)
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

		txids = append(txids, tid.TxID)
	}
	assert.Equal(t, []string{"txid12", "txid21", "txid100", "txid200", "txid1025"}, txids)

	it, err = store.Iterator(&driver.SeekSet{TxIDs: []string{"txid1025", "txid999", "txid21"}})

	assert.NoError(t, err)
	var results []*driver.ByNum[vc]
	for {
		tid, err := it.Next()
		assert.NoError(t, err)
		if tid == nil {
			it.Close()
			break
		}

		results = append(results, tid)
	}
	results = removeNils(results)
	txids = make([]string, len(results))
	for i, result := range results {
		txids[i] = result.TxID
	}
	assert.Len(t, txids, 2)
	assert.Equal(t, []string{"txid1025", "txid21"}, txids)
}
