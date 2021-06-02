/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package mem

import (
	"testing"
	"unicode/utf8"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
	"github.com/test-go/testify/assert"
	"google.golang.org/protobuf/proto"
)

func marshalOrPanic(o proto.Message) []byte {
	data, err := proto.Marshal(o)
	if err != nil {
		panic(err)
	}
	return data
}

var tempDir string

func TestRangeQueries1(t *testing.T) {
	ns := "namespace"

	db := New()

	err := db.BeginUpdate()
	assert.NoError(t, err)
	assert.Equal(t, db.keys, db.txn)

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

func TestMeta(t *testing.T) {
	ns := "ns"
	key := "key"

	db := New()

	err := db.BeginUpdate()
	assert.NoError(t, err)
	assert.Equal(t, db.keys, db.txn)

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
	assert.Equal(t, db.keys, db.txn)

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

	db := New()

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
	assert.Equal(t, db.keys, db.txn)

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
	assert.Equal(t, db.keys, db.txn)

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
	assert.Equal(t, db.keys, db.txn)

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
	assert.Equal(t, db.keys, db.txn)

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

func populateDB(t *testing.T, ns, key, keyWithSuffix string) *db {
	db := New()

	err := db.BeginUpdate()
	assert.NoError(t, err)
	assert.Equal(t, db.keys, db.txn)

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

	db := New()

	v, bn, tn, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Nil(t, v)
	assert.Equal(t, uint64(0x0), bn)
	assert.Equal(t, uint64(0x0), tn)
}

func TestMetadata(t *testing.T) {
	ns := "namespace"
	key := "foo"

	db := New()

	md, bn, txn, err := db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Nil(t, md)
	assert.Equal(t, uint64(0x0), bn)
	assert.Equal(t, uint64(0x0), txn)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	assert.Equal(t, db.keys, db.txn)

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
	assert.Equal(t, db.keys, db.txn)

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

	db := populateDB(t, ns, key, keyWithSuffix)

	err := db.BeginUpdate()
	assert.NoError(t, err)
	assert.Equal(t, db.keys, db.txn)

	err = db.DeleteState(ns, keyWithSuffix)
	assert.NoError(t, err)

	err = db.DeleteState(ns, key)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	assert.Len(t, db.keys, 0)
}

func TestDB2(t *testing.T) {
	ns := "namespace"
	key := "foo"
	keyWithSuffix := key + "/suffix"

	db := populateDB(t, ns, key, keyWithSuffix)

	err := db.BeginUpdate()
	assert.NoError(t, err)
	assert.Equal(t, db.keys, db.txn)

	err = db.DeleteState(ns, key)
	assert.NoError(t, err)

	err = db.DeleteState(ns, keyWithSuffix)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	assert.Len(t, db.keys, 0)
}

func TestRangeQueries(t *testing.T) {
	ns := "namespace"

	db := New()

	err := db.BeginUpdate()
	assert.NoError(t, err)
	assert.Equal(t, db.keys, db.txn)

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
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
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
	ck := compositeKeyNamespace + objectType + string(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + string(minUnicodeRuneValue)
	}
	return ck, nil
}

func TestCompositeKeys(t *testing.T) {
	ns := "namespace"
	keyPrefix := "prefix"

	db := New()

	err := db.BeginUpdate()
	assert.NoError(t, err)
	assert.Equal(t, db.keys, db.txn)

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
		{Key: "\x00prefix\x00a\x00b\x00", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x0, 0x61, 0x0, 0x62, 0x0}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix\x00a\x00b\x001\x00", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x0, 0x61, 0x0, 0x62, 0x0, 0x31, 0x0}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix\x00a\x00b\x003\x00", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x0, 0x61, 0x0, 0x62, 0x0, 0x33, 0x0}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix\x00a\x00d\x00", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x0, 0x61, 0x0, 0x64, 0x0}, Block: 0x23, IndexInBlock: 1},
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
		{Key: "\x00prefix\x00a\x00b\x00", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x0, 0x61, 0x0, 0x62, 0x0}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix\x00a\x00b\x001\x00", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x0, 0x61, 0x0, 0x62, 0x0, 0x31, 0x0}, Block: 0x23, IndexInBlock: 1},
		{Key: "\x00prefix\x00a\x00b\x003\x00", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x0, 0x61, 0x0, 0x62, 0x0, 0x33, 0x0}, Block: 0x23, IndexInBlock: 1},
	}, res)
}
