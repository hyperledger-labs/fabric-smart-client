/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dgraph-io/badger/v3"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger/mock"
	dbproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func marshalOrPanic(o proto.Message) []byte {
	data, err := proto.Marshal(o)
	if err != nil {
		panic(err)
	}
	return data
}

var tempDir string

func TestRangeQueries(t *testing.T) {
	ns := "namespace"

	dbpath := filepath.Join(tempDir, "DB-TestRangeQueries")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, "k2", []byte("k2_value"), 35, 1)
	assert.NoError(t, err)
	err = db.SetState(ns, "k3", []byte("k3_value"), 35, 2)
	assert.NoError(t, err)
	err = db.SetState(ns, "k1", []byte("k1_value"), 35, 3)
	assert.NoError(t, err)
	err = db.SetState(ns, "k111", []byte("k111_value"), 35, 4)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	itr, err := db.GetStateRangeScanIterator(ns, "", "")
	defer itr.Close()
	assert.NoError(t, err)

	res := make([]driver.VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Block: 35, IndexInBlock: 3},
		{Key: "k111", Raw: []byte("k111_value"), Block: 35, IndexInBlock: 4},
		{Key: "k2", Raw: []byte("k2_value"), Block: 35, IndexInBlock: 1},
		{Key: "k3", Raw: []byte("k3_value"), Block: 35, IndexInBlock: 2},
	}, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	defer itr.Close()
	assert.NoError(t, err)

	res = make([]driver.VersionedRead, 0, 3)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	expected := []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Block: 35, IndexInBlock: 3},
		{Key: "k111", Raw: []byte("k111_value"), Block: 35, IndexInBlock: 4},
		{Key: "k2", Raw: []byte("k2_value"), Block: 35, IndexInBlock: 1},
	}
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	defer itr.Close()
	assert.NoError(t, err)

	expected = []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Block: 35, IndexInBlock: 3},
		{Key: "k111", Raw: []byte("k111_value"), Block: 35, IndexInBlock: 4},
	}
	res = make([]driver.VersionedRead, 0, 2)
	for i := 0; i < 2; i++ {
		n, err := itr.Next()
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 2)
	assert.Equal(t, expected, res)

	expected = []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Block: 35, IndexInBlock: 3},
		{Key: "k111", Raw: []byte("k111_value"), Block: 35, IndexInBlock: 4},
		{Key: "k2", Raw: []byte("k2_value"), Block: 35, IndexInBlock: 1},
	}
	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	defer itr.Close()
	assert.NoError(t, err)

	res = make([]driver.VersionedRead, 0, 3)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)
}

func TestMarshallingErrors(t *testing.T) {
	ns := "ns"
	key := "key"

	dbpath := filepath.Join(tempDir, "DB-TestMarshallingErrors")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	txn := db.db.NewTransaction(true)

	err = txn.Set([]byte(ns+keys.NamespaceSeparator+key), []byte("barfobarf"))
	assert.NoError(t, err)

	err = txn.Commit()
	assert.NoError(t, err)

	v, bn, tn, err := db.GetState(ns, key)
	assert.Contains(t, err.Error(), "could not unmarshal VersionedValue for key ")
	assert.Equal(t, []byte(nil), v)
	assert.Equal(t, uint64(0), bn)
	assert.Equal(t, uint64(0), tn)

	m, bn, tn, err := db.GetStateMetadata(ns, key)
	assert.Contains(t, err.Error(), "could not unmarshal VersionedValue for key")
	assert.Len(t, m, 0)
	assert.Equal(t, uint64(0), bn)
	assert.Equal(t, uint64(0), tn)

	txn = db.db.NewTransaction(true)

	err = txn.Set([]byte(ns+keys.NamespaceSeparator+key), marshalOrPanic(&dbproto.VersionedValue{
		Version: 34,
	}))
	assert.NoError(t, err)

	err = txn.Commit()
	assert.NoError(t, err)

	v, bn, tn, err = db.GetState(ns, key)
	assert.EqualError(t, err, "could not get value for key ns\x00key: invalid version, expected 1, got 34")
	assert.Equal(t, []byte(nil), v)
	assert.Equal(t, uint64(0), bn)
	assert.Equal(t, uint64(0), tn)

	m, bn, tn, err = db.GetStateMetadata(ns, key)
	assert.EqualError(t, err, "could not get value for key ns\x00key: invalid version, expected 1, got 34")
	assert.Len(t, m, 0)
	assert.Equal(t, uint64(0), bn)
	assert.Equal(t, uint64(0), tn)
}

func TestMeta(t *testing.T) {
	ns := "ns"
	key := "key"

	dbpath := filepath.Join(tempDir, "DB-TestMeta")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, []byte("val"), 35, 1)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	v, bn, tn, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)
	assert.Equal(t, uint64(35), bn)
	assert.Equal(t, uint64(1), tn)

	m, bn, tn, err := db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Len(t, m, 0)
	assert.Equal(t, uint64(35), bn)
	assert.Equal(t, uint64(1), tn)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetStateMetadata(ns, key, map[string][]byte{"foo": []byte("bar")}, 36, 2)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	v, bn, tn, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)
	assert.Equal(t, uint64(36), bn)
	assert.Equal(t, uint64(2), tn)

	m, bn, tn, err = db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"foo": []byte("bar")}, m)
	assert.Equal(t, uint64(36), bn)
	assert.Equal(t, uint64(2), tn)
}

func TestSimpleReadWrite(t *testing.T) {
	ns := "ns"
	key := "key"

	dbpath := filepath.Join(tempDir, "DB-TestSimpleReadWrite")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	v, bn, tn, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)
	assert.Equal(t, uint64(0), bn)
	assert.Equal(t, uint64(0), tn)

	m, bn, tn, err := db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Len(t, m, 0)
	assert.Equal(t, uint64(0), bn)
	assert.Equal(t, uint64(0), tn)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, []byte("val"), 35, 1)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	v, bn, tn, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)
	assert.Equal(t, uint64(35), bn)
	assert.Equal(t, uint64(1), tn)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, []byte("val1"), 36, 2)
	assert.NoError(t, err)

	v, bn, tn, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)
	assert.Equal(t, uint64(35), bn)
	assert.Equal(t, uint64(1), tn)

	err = db.Commit()
	assert.NoError(t, err)

	v, bn, tn, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)
	assert.Equal(t, uint64(36), bn)
	assert.Equal(t, uint64(2), tn)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, []byte("val0"), 37, 3)
	assert.NoError(t, err)

	err = db.Discard()
	assert.NoError(t, err)

	v, bn, tn, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)
	assert.Equal(t, uint64(36), bn)
	assert.Equal(t, uint64(2), tn)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.DeleteState(ns, key)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	v, bn, tn, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)
	assert.Equal(t, uint64(0), bn)
	assert.Equal(t, uint64(0), tn)
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

func populateDB(t *testing.T, ns, key, keyWithSuffix, dbname string) *badgerDB {
	dbpath := filepath.Join(tempDir, dbname)
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	assert.NoError(t, err)
	assert.NotNil(t, db)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, []byte("bar"), 1, 1)
	assert.NoError(t, err)

	err = db.SetState(ns, keyWithSuffix, []byte("bar1"), 1, 1)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	val, block, txnum, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, val, []byte("bar"))
	assert.Equal(t, block, uint64(1))
	assert.Equal(t, txnum, uint64(1))

	val, block, txnum, err = db.GetState(ns, keyWithSuffix)
	assert.NoError(t, err)
	assert.Equal(t, val, []byte("bar1"))
	assert.Equal(t, block, uint64(1))
	assert.Equal(t, txnum, uint64(1))

	val, block, txnum, err = db.GetState(ns, "barf")
	assert.NoError(t, err)
	assert.Nil(t, val)
	assert.Equal(t, block, uint64(0))
	assert.Equal(t, txnum, uint64(0))

	val, block, txnum, err = db.GetState("barf", "barf")
	assert.NoError(t, err)
	assert.Nil(t, val)
	assert.Equal(t, block, uint64(0))
	assert.Equal(t, txnum, uint64(0))

	return db
}

func TestGetNonExistent(t *testing.T) {
	ns := "namespace"
	key := "foo"

	dbpath := filepath.Join(tempDir, "TestGetNonExistent")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	v, bn, tn, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Nil(t, v)
	assert.Equal(t, uint64(0x0), bn)
	assert.Equal(t, uint64(0x0), tn)
}

func TestMetadata(t *testing.T) {
	ns := "namespace"
	key := "foo"

	dbpath := filepath.Join(tempDir, "TestMetadata")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	md, bn, txn, err := db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Nil(t, md)
	assert.Equal(t, uint64(0x0), bn)
	assert.Equal(t, uint64(0x0), txn)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetStateMetadata(ns, key, map[string][]byte{"foo": []byte("bar")}, 35, 1)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	md, bn, txn, err = db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"foo": []byte("bar")}, md)
	assert.Equal(t, uint64(35), bn)
	assert.Equal(t, uint64(1), txn)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetStateMetadata(ns, key, map[string][]byte{"foo1": []byte("bar1")}, 36, 2)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	md, bn, txn, err = db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"foo1": []byte("bar1")}, md)
	assert.Equal(t, uint64(36), bn)
	assert.Equal(t, uint64(2), txn)
}

func TestDB1(t *testing.T) {
	ns := "namespace"
	key := "foo"
	keyWithSuffix := key + "/suffix"

	db := populateDB(t, ns, key, keyWithSuffix, "TestDB1")

	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.DeleteState(ns, keyWithSuffix)
	assert.NoError(t, err)

	err = db.DeleteState(ns, key)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)
}

func TestDB2(t *testing.T) {
	ns := "namespace"
	key := "foo"
	keyWithSuffix := key + "/suffix"

	db := populateDB(t, ns, key, keyWithSuffix, "TestDB2")

	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.DeleteState(ns, key)
	assert.NoError(t, err)

	err = db.DeleteState(ns, keyWithSuffix)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)
}

func TestRangeQueries1(t *testing.T) {
	ns := "namespace"

	dbpath := filepath.Join(tempDir, "TestRangeQueries1")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, "k2", []byte("k2_value"), 35, 1)
	assert.NoError(t, err)
	err = db.SetState(ns, "k3", []byte("k3_value"), 35, 2)
	assert.NoError(t, err)
	err = db.SetState(ns, "k1", []byte("k1_value"), 35, 3)
	assert.NoError(t, err)
	err = db.SetState(ns, "k111", []byte("k111_value"), 35, 4)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	itr, err := db.GetStateRangeScanIterator(ns, "", "")
	defer itr.Close()
	assert.NoError(t, err)

	res := make([]driver.VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Block: 35, IndexInBlock: 3},
		{Key: "k111", Raw: []byte("k111_value"), Block: 35, IndexInBlock: 4},
		{Key: "k2", Raw: []byte("k2_value"), Block: 35, IndexInBlock: 1},
		{Key: "k3", Raw: []byte("k3_value"), Block: 35, IndexInBlock: 2},
	}, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	defer itr.Close()
	assert.NoError(t, err)

	res = make([]driver.VersionedRead, 0, 3)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Block: 35, IndexInBlock: 3},
		{Key: "k111", Raw: []byte("k111_value"), Block: 35, IndexInBlock: 4},
		{Key: "k2", Raw: []byte("k2_value"), Block: 35, IndexInBlock: 1},
	}, res)
}

const (
	minUnicodeRuneValue   = 0            // U+0000
	maxUnicodeRuneValue   = utf8.MaxRune // U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
)

func validateCompositeKeyAttribute(str string) error {
	if !utf8.ValidString(str) {
		return errors.Errorf("not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return errors.Errorf(`input contain unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}

func createCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	ck := compositeKeyNamespace + objectType + fmt.Sprint(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + fmt.Sprint(minUnicodeRuneValue)
	}
	return ck, nil
}

func TestCompositeKeys(t *testing.T) {
	ns := "namespace"
	keyPrefix := "prefix"

	dbpath := filepath.Join(tempDir, "TestCompositeKeys")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	defer db.Close()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	for _, comps := range [][]string{
		{"a", "b", "1"},
		{"a", "b"},
		{"a", "b", "3"},
		{"a", "d"},
	} {
		k, err := createCompositeKey(keyPrefix, comps)
		assert.NoError(t, err)
		err = db.SetState(ns, k, []byte(k), 35, 1)
		assert.NoError(t, err)
	}

	err = db.Commit()
	assert.NoError(t, err)

	partialCompositeKey, err := createCompositeKey(keyPrefix, []string{"a"})
	assert.NoError(t, err)
	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	itr, err := db.GetStateRangeScanIterator(ns, startKey, endKey)
	defer itr.Close()
	assert.NoError(t, err)

	res := make([]driver.VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "\x00prefix0a0b0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix0a0b010", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x31, 0x30}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix0a0b030", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x33, 0x30}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix0a0d0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x64, 0x30}, Block: 0x23, IndexInBlock: 1},
	}, res)

	partialCompositeKey, err = createCompositeKey(keyPrefix, []string{"a", "b"})
	assert.NoError(t, err)
	startKey = partialCompositeKey
	endKey = partialCompositeKey + string(maxUnicodeRuneValue)

	itr, err = db.GetStateRangeScanIterator(ns, startKey, endKey)
	defer itr.Close()
	assert.NoError(t, err)

	res = make([]driver.VersionedRead, 0, 2)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "\x00prefix0a0b0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix0a0b010", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x31, 0x30}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix0a0b030", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x33, 0x30}, Block: 0x23, IndexInBlock: 1},
	}, res)
}

func TestAutoCleaner(t *testing.T) {
	dbpath := filepath.Join(tempDir, "DB-autocleaner")

	// if db is nil should return nil
	cancel := autoCleaner(nil, defaultGCInterval, defaultGCDiscardRatio)
	assert.Nil(t, cancel)

	// no need to run auto clean if we use in memory badger
	opt := badger.DefaultOptions("").WithInMemory(true)
	db, err := badger.Open(opt)
	assert.NoError(t, err)

	cancel = autoCleaner(db, defaultGCInterval, defaultGCDiscardRatio)
	assert.Nil(t, cancel)

	err = db.Close()
	assert.NoError(t, err)

	// let's see if we get our auto cleaner running
	opt = badger.DefaultOptions(dbpath)
	db, err = badger.Open(opt)
	assert.NoError(t, err)

	cancel = autoCleaner(db, defaultGCInterval, defaultGCDiscardRatio)
	assert.NotNil(t, cancel)

	cancel()
	// let's call it again to make sure we do not panic
	cancel()

	err = db.Close()
	assert.NoError(t, err)

	// let's see if we get our auto cleaner running
	opt = badger.DefaultOptions(dbpath)
	db, err = badger.Open(opt)
	assert.NoError(t, err)

	cancel = autoCleaner(db, defaultGCInterval, defaultGCDiscardRatio)
	assert.NotNil(t, cancel)
	err = db.Close()
	assert.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	// cancel the auto cleaner after the db was closed already
	cancel()
	// no panic
}

func TestAutoCleanerWithMock(t *testing.T) {

	// let's assume db is already closed
	db := &mock.BadgerDB{}
	db.OptsReturns(badger.DefaultOptions(""))
	db.IsClosedReturns(true)
	_ = autoCleaner(db, 10*time.Millisecond, defaultGCDiscardRatio)
	// wait a bit
	time.Sleep(200 * time.Millisecond)
	// the ticker should have ticked only once
	assert.Equal(t, 1, db.IsClosedCallCount())

	// let's cancel before we tick first times
	db = &mock.BadgerDB{}
	db.OptsReturns(badger.DefaultOptions(""))
	db.IsClosedReturns(false)
	cancel := autoCleaner(db, 1*time.Minute, defaultGCDiscardRatio)
	// wait a bit
	time.Sleep(200 * time.Millisecond)
	cancel()
	// the ticker should have ticked only once
	assert.Equal(t, 0, db.IsClosedCallCount())

	// let's cancel before we tick first times
	db = &mock.BadgerDB{}
	db.OptsReturns(badger.DefaultOptions(""))
	db.IsClosedReturns(false)
	// even errors should not prevent us from running our cleaner
	db.RunValueLogGCReturnsOnCall(1, nil)
	db.RunValueLogGCReturnsOnCall(2, badger.ErrRejected)
	db.RunValueLogGCReturnsOnCall(3, badger.ErrNoRewrite)
	db.RunValueLogGCReturnsOnCall(4, errors.New("some error"))
	cancel = autoCleaner(db, 10*time.Millisecond, defaultGCDiscardRatio)
	// wait a bit
	time.Sleep(200 * time.Millisecond)
	cancel()
	// the ticker should have ticked a couple of times
	// actually we would assume that the ticker is called ~20 times,
	// however, as timing could make this a flacky test we just check conservatively
	assert.GreaterOrEqual(t, db.RunValueLogGCCallCount(), 4)
}

var (
	namespace = "test_namespace"
	key       = "test_key"
)

var result string

func BenchmarkConcatenation(b *testing.B) {
	var s string
	for i := 0; i < b.N; i++ {
		s = namespace + keys.NamespaceSeparator + key
	}
	result = s
}

func BenchmarkBuilder(b *testing.B) {
	var s string
	for i := 0; i < b.N; i++ {
		var sb strings.Builder
		sb.WriteString(namespace)
		sb.WriteString(keys.NamespaceSeparator)
		sb.WriteString(key)
		s = sb.String()
	}
	result = s
}
