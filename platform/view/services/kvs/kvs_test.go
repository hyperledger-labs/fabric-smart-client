/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs_test

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	registry2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/registry"
)

type fakeProv struct {
	typ  string
	path string
}

func (f *fakeProv) GetString(key string) string {
	return f.typ
}

func (f *fakeProv) GetDuration(key string) time.Duration {
	return time.Duration(0)
}

func (f *fakeProv) GetBool(key string) bool {
	return false
}

func (f *fakeProv) GetStringSlice(key string) []string {
	return nil
}

func (f *fakeProv) IsSet(key string) bool {
	return false
}

func (f *fakeProv) UnmarshalKey(key string, rawVal interface{}) error {
	*(rawVal.(*kvs.Opts)) = kvs.Opts{
		Path: f.path,
	}

	return nil
}

func (f *fakeProv) ConfigFileUsed() string {
	return ""
}

func (f *fakeProv) GetPath(key string) string {
	return ""
}

func (f *fakeProv) TranslatePath(path string) string {
	return ""
}

type stuff struct {
	S string `json:"s"`
	I int    `json:"i"`
}

func testRound(t *testing.T, cfg *fakeProv) {
	registry := registry2.New()
	registry.RegisterService(cfg)

	kvstore, err := kvs.New(cfg.typ, "_default", registry)
	assert.NoError(t, err)

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
		_, err = it.Next(val)
		assert.NoError(t, err)
		if ctr == 0 {
			assert.Equal(t, &stuff{"santa", 1}, val)
		} else if ctr == 1 {
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
		_, err = it.Next(val)
		assert.NoError(t, err)
		if ctr == 0 {
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

func TestKVS(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "kvstest-*")
	assert.NoError(t, err)
	defer os.RemoveAll(path)
	testRound(t, &fakeProv{typ: "memory"})
	testRound(t, &fakeProv{typ: "badger", path: path})
}
