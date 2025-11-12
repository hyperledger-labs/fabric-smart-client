/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	kvs2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/stretchr/testify/assert"
)

var sanitizer = postgres2.NewSanitizer()

type stuff struct {
	S string `json:"s"`
	I int    `json:"i"`
}

func testRound(t *testing.T, driver driver.Driver) {
	kvstore, err := kvs2.New(utils.MustGet(driver.NewKVS("")), "_default", kvs2.DefaultCacheSize)
	assert.NoError(t, err)
	defer kvstore.Stop()

	k1, err := createCompositeKey("k", []string{"1"})
	assert.NoError(t, err)
	k2, err := createCompositeKey("k", []string{"2"})
	assert.NoError(t, err)

	err = kvstore.Put(context.Background(), k1, &stuff{"santa", 1})
	assert.NoError(t, err)

	val := &stuff{}
	err = kvstore.Get(context.Background(), k1, val)
	assert.NoError(t, err)
	assert.Equal(t, &stuff{"santa", 1}, val)

	err = kvstore.Put(context.Background(), k2, &stuff{"claws", 2})
	assert.NoError(t, err)

	val = &stuff{}
	err = kvstore.Get(context.Background(), k2, val)
	assert.NoError(t, err)
	assert.Equal(t, &stuff{"claws", 2}, val)

	it, err := kvstore.GetByPartialCompositeID(context.Background(), "k", []string{})
	assert.NoError(t, err)
	defer utils.IgnoreErrorFunc(it.Close)

	for ctr := 0; it.HasNext(); ctr++ {
		val = &stuff{}
		key, err := it.Next(val)
		assert.NoError(t, err)
		switch ctr {
		case 0:
			assert.Equal(t, k1, key)
			assert.Equal(t, &stuff{"santa", 1}, val)
		case 1:
			assert.Equal(t, k2, key)
			assert.Equal(t, &stuff{"claws", 2}, val)
		default:
			assert.Fail(t, "expected 2 entries in the range, found more")
		}
	}

	assert.NoError(t, kvstore.Delete(context.Background(), k2))
	assert.False(t, kvstore.Exists(context.Background(), k2))
	val = &stuff{}
	err = kvstore.Get(context.Background(), k2, val)
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
	h := sha256.Sum256([]byte("Hello World"))
	k, err := sanitizer.Encode(string(h[:]))
	assert.NoError(t, err)
	assert.NoError(t, kvstore.Put(context.Background(), k, val))
	assert.True(t, kvstore.Exists(context.Background(), k))
	val2 := &stuff{}
	assert.NoError(t, kvstore.Get(context.Background(), k, val2))
	assert.Equal(t, val, val2)
	assert.NoError(t, kvstore.Delete(context.Background(), k))
	assert.False(t, kvstore.Exists(context.Background(), k))
}

func createCompositeKey(objectType string, attributes []string) (string, error) {
	k, err := kvs2.CreateCompositeKey(objectType, attributes)
	if err != nil {
		return "", err
	}
	return sanitizer.Encode(k)
}

func testParallelWrites(t *testing.T, driver driver.Driver) {
	kvstore, err := kvs2.New(utils.MustGet(driver.NewKVS("")), "_default", kvs2.DefaultCacheSize)
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
			err = kvstore.Put(context.Background(), k1, &stuff{"santa", i})
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
			err := kvstore.Put(context.Background(), k1, &stuff{"santa", 1})
			assert.NoError(t, err)
			defer wg.Done()
		}(i)
	}
	wg.Wait()
}

func TestMemoryKVS(t *testing.T) {
	testRound(t, mem.NewDriver())
	testParallelWrites(t, mem.NewDriver())
}

func TestSQLiteKVS(t *testing.T) {
	cp := multiplexed.MockTypeConfig(sqlite2.Persistence, sqlite2.Config{
		DataSource:      fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(t.TempDir(), "sqlite_test")),
		TablePrefix:     "",
		SkipCreateTable: false,
		SkipPragmas:     false,
		MaxOpenConns:    0,
		MaxIdleConns:    common.CopyPtr(2),
		MaxIdleTime:     common.CopyPtr(time.Minute),
	})
	testRound(t, sqlite2.NewDriver(cp))
	testParallelWrites(t, sqlite2.NewDriver(cp))
}

func TestPostgresKVS(t *testing.T) {
	t.Log("starting postgres")
	terminate, pgConnStr, err := postgres2.StartPostgres(t.Context(), postgres2.ConfigFromEnv(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(terminate)
	t.Log("postgres ready")

	cp := multiplexed.MockTypeConfig(postgres2.Persistence, postgres2.Config{
		DataSource:      pgConnStr,
		TablePrefix:     "",
		SkipCreateTable: false,

		MaxOpenConns: 50,
		MaxIdleConns: common.CopyPtr(10),
		MaxIdleTime:  common.CopyPtr(time.Minute),
	})

	testRound(t, postgres2.NewDriver(cp))
	testParallelWrites(t, postgres2.NewDriver(cp))
}
