/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs_test

import (
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/stretchr/testify/assert"
)

var sanitizer = postgres.NewSanitizer()

type stuff struct {
	S string `json:"s"`
	I int    `json:"i"`
}

func testRound(t *testing.T, driver driver.Driver) {
	kvstore, err := kvs.New(utils.MustGet(driver.NewKVS("")), "_default", kvs.DefaultCacheSize)
	assert.NoError(t, err)
	defer kvstore.Stop()

	k1, err := createCompositeKey("k", []string{"1"})
	assert.NoError(t, err)
	k2, err := createCompositeKey("k", []string{"2"})
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
	k, err := sanitizer.Encode(hash.Hashable("Hello World").RawString())
	assert.NoError(t, err)
	assert.NoError(t, kvstore.Put(k, val))
	assert.True(t, kvstore.Exists(k))
	val2 := &stuff{}
	assert.NoError(t, kvstore.Get(k, val2))
	assert.Equal(t, val, val2)
	assert.NoError(t, kvstore.Delete(k))
	assert.False(t, kvstore.Exists(k))
}

func createCompositeKey(objectType string, attributes []string) (string, error) {
	k, err := kvs.CreateCompositeKey(objectType, attributes)
	if err != nil {
		return "", err
	}
	return sanitizer.Encode(k)
}

func testParallelWrites(t *testing.T, driver driver.Driver) {
	kvstore, err := kvs.New(utils.MustGet(driver.NewKVS("")), "_default", kvs.DefaultCacheSize)
	assert.NoError(t, err)
	defer kvstore.Stop()

	// different keys
	wg := sync.WaitGroup{}
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			k1, err := createCompositeKey("parallel_key_1_", []string{fmt.Sprintf("%d", i)})
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
	k1, err := createCompositeKey("parallel_key_2_", []string{"1"})
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

func TestMemoryKVS(t *testing.T) {
	d := mem.NewDriver()
	testRound(t, d)
	testParallelWrites(t, d)
}

func TestSQLiteKVS(t *testing.T) {
	cp := multiplexed.MockTypeConfig(sqlite.Persistence, sqlite.Config{
		DataSource:      fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(t.TempDir(), "sqlite_test")),
		TablePrefix:     "",
		SkipCreateTable: false,
		SkipPragmas:     false,
		MaxOpenConns:    0,
		MaxIdleConns:    common.CopyPtr(2),
		MaxIdleTime:     common.CopyPtr(time.Minute),
	})
	d := sqlite.NewDriver(cp)
	testRound(t, d)
	testParallelWrites(t, d)
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

	cp := multiplexed.MockTypeConfig(postgres.Persistence, postgres.Config{
		DataSource:      pgConnStr,
		TablePrefix:     "",
		SkipCreateTable: false,

		MaxOpenConns: 50,
		MaxIdleConns: common.CopyPtr(10),
		MaxIdleTime:  common.CopyPtr(time.Minute),
	})

	d := postgres.NewDriver(cp)
	testRound(t, d)
	testParallelWrites(t, d)
}
