/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs_test

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"
	"github.com/stretchr/testify/assert"
)

type stuff struct {
	S string `json:"s"`
	I int    `json:"i"`
}

func testRound(t *testing.T, driver driver.Driver, cp kvs.ConfigProvider) {
	kvstore, err := kvs.NewWithConfig(driver, "_default", cp)
	assert.NoError(t, err)
	defer kvstore.Stop()

	k1, err := kvs.CreateCompositeKey("k", []string{"1"})
	assert.NoError(t, err)
	k2, err := kvs.CreateCompositeKey("k", []string{"2"})
	assert.NoError(t, err)

	err = kvstore.Put(k1, &stuff{"santa", 1})
	assert.NoError(t, err)

	val := &stuff{}
	err = kvstore.Get(k1, val)
	assert.NoError(t, err)
	assert.Equal(t, &stuff{"santa", 1}, val)

	err = kvstore.Put(k2, &stuff{"claws", 2})
	assert.NoError(t, err)

	val = &stuff{}
	err = kvstore.Get(k2, val)
	assert.NoError(t, err)
	assert.Equal(t, &stuff{"claws", 2}, val)

	it, err := kvstore.GetByPartialCompositeID("k", []string{})
	assert.NoError(t, err)
	defer it.Close()

	for ctr := 0; it.HasNext(); ctr++ {
		val = &stuff{}
		key, err := it.Next(val)
		assert.NoError(t, err)
		if ctr == 0 {
			assert.Equal(t, k1, key)
			assert.Equal(t, &stuff{"santa", 1}, val)
		} else if ctr == 1 {
			assert.Equal(t, k2, key)
			assert.Equal(t, &stuff{"claws", 2}, val)
		} else {
			assert.Fail(t, "expected 2 entries in the range, found more")
		}
	}

	assert.NoError(t, kvstore.Delete(k2))
	assert.False(t, kvstore.Exists(k2))
	val = &stuff{}
	err = kvstore.Get(k2, val)
	assert.Error(t, err)

	for ctr := 0; it.HasNext(); ctr++ {
		val = &stuff{}
		key, err := it.Next(val)
		assert.NoError(t, err)
		if ctr == 0 {
			assert.Equal(t, k1, key)
			assert.Equal(t, &stuff{"santa", 1}, val)
		} else {
			assert.Fail(t, "expected 2 entries in the range, found more")
		}
	}

	val = &stuff{
		S: "hello",
		I: 100,
	}
	k := hash.Hashable("Hello World").RawString()
	assert.NoError(t, kvstore.Put(k, val))
	assert.True(t, kvstore.Exists(k))
	val2 := &stuff{}
	assert.NoError(t, kvstore.Get(k, val2))
	assert.Equal(t, val, val2)
	assert.NoError(t, kvstore.Delete(k))
	assert.False(t, kvstore.Exists(k))
}

func testParallelWrites(t *testing.T, driver driver.Driver, cp kvs.ConfigProvider) {
	kvstore, err := kvs.NewWithConfig(driver, "_default", cp)
	assert.NoError(t, err)
	defer kvstore.Stop()

	// different keys
	wg := sync.WaitGroup{}
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			k1, err := kvs.CreateCompositeKey("parallel_key_1_", []string{fmt.Sprintf("%d", i)})
			assert.NoError(t, err)
			err = kvstore.Put(k1, &stuff{"santa", i})
			assert.NoError(t, err)
			defer wg.Done()
		}(i)
	}
	wg.Wait()

	// same key
	wg = sync.WaitGroup{}
	wg.Add(n)
	k1, err := kvs.CreateCompositeKey("parallel_key_2_", []string{"1"})
	assert.NoError(t, err)
	for i := 0; i < n; i++ {
		go func(i int) {
			err := kvstore.Put(k1, &stuff{"santa", 1})
			assert.NoError(t, err)
			defer wg.Done()
		}(i)
	}
	wg.Wait()
}

func TestBadgerKVS(t *testing.T) {
	path, err := os.MkdirTemp(os.TempDir(), "kvstest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(path)

	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyStub = func(s string, i interface{}) error {
		_, ok := i.(*badger.Opts)
		if ok {
			*(i.(*badger.Opts)) = badger.Opts{
				Path: path,
			}
		}
		return nil
	}
	cp.IsSetReturns(false)
	d := &badger.Driver{}
	testRound(t, d, cp)
	testParallelWrites(t, d, cp)
}

func TestMemoryKVS(t *testing.T) {
	cp := &mock.ConfigProvider{}
	d := &mem.Driver{}
	testRound(t, d, cp)
	testParallelWrites(t, d, cp)
}

func TestSQLiteKVS(t *testing.T) {
	cp := &mock.ConfigProvider{}
	d := &sqlite.TestDriver{
		Name:    "sqlite_test",
		TempDir: t.TempDir(),
	}
	testRound(t, d, cp)
	testParallelWrites(t, d, cp)
}

func TestPostgresKVS(t *testing.T) {
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("set environment variable TEST_POSTGRES to true to include postgres test")
	}
	if testing.Short() {
		t.Skip("skipping postgres test in short mode")
	}
	t.Log("starting postgres")
	terminate, pgConnStr, err := postgres.StartPostgres(t, false)
	if err != nil {
		t.Fatal(err)
	}
	defer terminate()
	t.Log("postgres ready")

	cp := &mock.ConfigProvider{}
	d := &postgres.TestDriver{
		Name:    "hw",
		ConnStr: pgConnStr,
	}
	testRound(t, d, cp)
	testParallelWrites(t, d, cp)
}
