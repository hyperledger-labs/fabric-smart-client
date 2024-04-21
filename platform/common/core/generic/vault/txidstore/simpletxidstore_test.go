/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package txidstore

import (
	"os"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/pkg/errors"
	"github.com/test-go/testify/assert"
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

func TestTXIDStoreMem(t *testing.T) {
	db, err := db.Open(nil, "memory", "", nil)
	assert.NoError(t, err)
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

func TestTXIDStoreBadger(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "TestTXIDStoreBadger")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	c.IsSetReturns(false)
	db, err := db.Open(nil, "badger", tempDir, c)
	assert.NoError(t, err)
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

	it, err := store.Iterator(&vault.SeekStart{})
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

	it, err = store.Iterator(&vault.SeekEnd{})
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
	it, err = store.Iterator(&vault.SeekStart{})
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
	it, err = store.Iterator(&vault.SeekStart{})
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

	it, err := store.Iterator(&vault.SeekEnd{})
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
	err = store.Set("txid1025", valid, "")
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

	it, err = store.Iterator(&vault.SeekEnd{})
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

	it, err = store.Iterator(&vault.SeekStart{})
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

	it, err = store.Iterator(&vault.SeekPos{Txid: "boh"})
	assert.EqualError(t, err, "txid boh was not found")

	it, err = store.Iterator(&vault.SeekPos{Txid: "txid12"})
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
}
