/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package unversioned_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned/mocks"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mocks/config.go -fake-name Config . config

type config interface {
	db.Config
}

var tempDir string

func TestRangeQueriesBadger(t *testing.T) {
	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	c.IsSetReturns(false)
	dbpath := filepath.Join(tempDir, "DB-TestRangeQueries")
	db, err := db.Open(nil, "badger", dbpath, c)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	testRangeQueries(t, db)
}

func TestRangeQueriesMemory(t *testing.T) {
	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	db, err := db.Open(nil, "memory", "", c)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	testRangeQueries(t, db)
}

func testRangeQueries(t *testing.T, db driver.Persistence) {
	var err error

	ns := "namespace"

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, "k2", []byte("k2_value"))
	assert.NoError(t, err)
	err = db.SetState(ns, "k3", []byte("k3_value"))
	assert.NoError(t, err)
	err = db.SetState(ns, "k1", []byte("k1_value"))
	assert.NoError(t, err)
	err = db.SetState(ns, "k111", []byte("k111_value"))
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	itr, err := db.GetStateRangeScanIterator(ns, "", "")
	defer itr.Close()
	assert.NoError(t, err)

	res := make([]driver.Read, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.Read{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
		{Key: "k3", Raw: []byte("k3_value")},
	}, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	defer itr.Close()
	assert.NoError(t, err)

	res = make([]driver.Read, 0, 3)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.Read{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}, res)
}

func TestSimpleReadWriteBadger(t *testing.T) {
	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	dbpath := filepath.Join(tempDir, "DB-TestRangeQueries")
	db, err := db.Open(nil, "badger", dbpath, c)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	testSimpleReadWrite(t, db)
}

func TestSimpleReadWriteMemory(t *testing.T) {
	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	db, err := db.Open(nil, "memory", "", c)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	testSimpleReadWrite(t, db)
}

func testSimpleReadWrite(t *testing.T, db driver.Persistence) {
	ns := "ns"
	key := "key"

	v, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, []byte("val"))
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, []byte("val1"))
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, []byte("val0"))
	assert.NoError(t, err)

	err = db.Discard()
	assert.NoError(t, err)

	v, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.DeleteState(ns, key)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)
}

func TestMain(m *testing.M) {
	var err error
	tempDir, err = ioutil.TempDir("", "badger-fsc-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temporary directory: %v", err)
		os.Exit(-1)
	}
	defer os.RemoveAll(tempDir)

	m.Run()
}
