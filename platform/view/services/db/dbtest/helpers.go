/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dbtest

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/pkg/errors"
	"github.com/test-go/testify/assert"
)

// This file exposes functions that db drivers can use for integration tests
var Cases = []struct {
	Name string
	Fn   func(*testing.T, driver.TransactionalVersionedPersistence)
}{
	{"RangeQueries", TTestRangeQueries},
	{"Meta", TTestMeta},
	{"SimpleReadWrite", TTestSimpleReadWrite},
	{"GetNonExistent", TTestGetNonExistent},
	{"Metadata", TTestMetadata},
	{"DB1", TTestDB1},
	{"DB2", TTestDB2},
	{"RangeQueries1", TTestRangeQueries1},
	{"MultiWritesAndRangeQueries", TTestMultiWritesAndRangeQueries},
	{"TTestMultiWrites", TTestMultiWrites},
	{"CompositeKeys", TTestCompositeKeys},
}

var UnversionedCases = []struct {
	Name string
	Fn   func(*testing.T, driver.UnversionedPersistence)
}{
	{"UnversionedSimple", TTestUnversionedSimple},
	{"UnversionedRange", TTestUnversionedRange},
	{"NonUTF8keys", TTestNonUTF8keys},
}

var UnversionedNotifierCases = []struct {
	Name string
	Fn   func(*testing.T, driver.UnversionedNotifier)
}{
	{"UnversionedNotifierSimple", TTestUnversionedNotifierSimple},
}

var VersionedNotifierCases = []struct {
	Name string
	Fn   func(*testing.T, driver.VersionedNotifier)
}{
	{"VersionedNotifierSimple", TTestVersionedNotifierSimple},
}

var ErrorCases = []struct {
	Name string
	Fn   func(t *testing.T, readDB *sql.DB, writeDB *sql.DB, errorWrapper driver.SQLErrorWrapper, table string)
}{
	{"Duplicate", TTestDuplicate},
}

func TTestDuplicate(t *testing.T, _ *sql.DB, writeDB *sql.DB, errorWrapper driver.SQLErrorWrapper, table string) {
	ns := "namespace"

	tx, err := writeDB.Begin()
	assert.NoError(t, err, "should start tx")

	query := fmt.Sprintf("INSERT INTO %s (ns, pkey, val) VALUES ($1, $2, $3)", table)

	_, err = tx.Exec(query, ns, "key", []byte("test 1"))
	assert.NoError(t, err, "should insert first row")

	_, err = tx.Exec(query, ns, "key", []byte("test 2"))
	assert.Error(t, err, "should fail on duplicate")
	assert.True(t, errors2.HasCause(errorWrapper.WrapError(err), driver.UniqueKeyViolation), "should be a unique-key violation")

	err = tx.Rollback()
	assert.NoError(t, err, "should rollback")
}

func TTestRangeQueries(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"
	populateForRangeQueries(t, db, ns)

	itr, err := db.GetStateRangeScanIterator(ns, "", "")
	assert.NoError(t, err)
	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: ToBytes(35, 1)},
		{Key: "k3", Raw: []byte("k3_value"), Version: ToBytes(35, 2)},
	}, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	expected := []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: ToBytes(35, 1)},
	}
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadFirst(itr, 2)
	assert.NoError(t, err)
	expected = []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
	}
	assert.Len(t, res, 2)
	assert.Equal(t, expected, res)

	expected = []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: ToBytes(35, 1)},
	}
	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)

	expected = []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k3", Raw: []byte("k3_value"), Version: ToBytes(35, 2)},
	}
	itr, err = db.GetStateSetIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 2)
	for _, read := range expected {
		assert.Contains(t, res, read)
	}
}

func TTestMeta(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "ns"
	key := "key"

	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, driver.VersionedValue{Raw: []byte("val"), Version: ToBytes(35, 1)})
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	vv, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{Raw: []byte("val"), Version: ToBytes(35, 1)}, vv)

	m, ver, err := db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Len(t, m, 0)
	bn, tn, err := FromBytes(ver)
	assert.NoError(t, err)
	assert.Equal(t, uint64(35), bn)
	assert.Equal(t, uint64(1), tn)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetStateMetadata(ns, key, map[string][]byte{"foo": []byte("bar")}, ToBytes(36, 2))
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{Raw: []byte("val"), Version: ToBytes(36, 2)}, vv)

	m, ver, err = db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	bn, tn, err = FromBytes(ver)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"foo": []byte("bar")}, m)
	assert.Equal(t, uint64(36), bn)
	assert.Equal(t, uint64(2), tn)
}

func TTestSimpleReadWrite(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "ns"
	key := "key"

	// empty state
	vv, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{}, vv)

	// empty metadata
	m, ver, err := db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	bn, tn, err := FromBytes(ver)
	assert.NoError(t, err)
	assert.Len(t, m, 0)
	assert.Equal(t, uint64(0), bn)
	assert.Equal(t, uint64(0), tn)

	// add data
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(ns, key, driver.VersionedValue{Raw: []byte("val"), Version: ToBytes(35, 1)})
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	// get data
	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{Raw: []byte("val"), Version: ToBytes(35, 1)}, vv)

	// logging because this can cause a deadlock if maxOpenConnections is only 1
	t.Logf("get state [%s] during set state tx", key)
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(ns, key, driver.VersionedValue{Raw: []byte("val1"), Version: ToBytes(36, 2)})
	assert.NoError(t, err)

	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{Raw: []byte("val"), Version: ToBytes(35, 1)}, vv)
	err = db.Commit()
	assert.NoError(t, err)

	t.Logf("get state after tx [%s]", key)
	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{Raw: []byte("val1"), Version: ToBytes(36, 2)}, vv)

	// Discard an update
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(ns, key, driver.VersionedValue{Raw: []byte("val0"), Version: ToBytes(37, 3)})
	assert.NoError(t, err)
	err = db.Discard()
	assert.NoError(t, err)

	// Expect state to be same as before the rollback
	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{Raw: []byte("val1"), Version: ToBytes(36, 2)}, vv)

	// delete state
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.DeleteState(ns, key)
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	// expect state to be empty
	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{}, vv)
}

func populateDB(t *testing.T, db driver.TransactionalVersionedPersistence, ns, key, keyWithSuffix string) {
	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, driver.VersionedValue{Raw: []byte("bar"), Version: ToBytes(1, 1)})
	assert.NoError(t, err)

	err = db.SetState(ns, keyWithSuffix, driver.VersionedValue{Raw: []byte("bar1"), Version: ToBytes(1, 1)})
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	vv, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{Raw: []byte("bar"), Version: ToBytes(1, 1)}, vv)

	vv, err = db.GetState(ns, keyWithSuffix)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{Raw: []byte("bar1"), Version: ToBytes(1, 1)}, vv)

	vv, err = db.GetState(ns, "barf")
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{}, vv)

	vv, err = db.GetState("barf", "barf")
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{}, vv)
}

func populateForRangeQueries(t *testing.T, db driver.TransactionalVersionedPersistence, ns string) {
	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, "k2", driver.VersionedValue{Raw: []byte("k2_value"), Version: ToBytes(35, 1)})
	assert.NoError(t, err)
	err = db.SetState(ns, "k3", driver.VersionedValue{Raw: []byte("k3_value"), Version: ToBytes(35, 2)})
	assert.NoError(t, err)
	err = db.SetState(ns, "k1", driver.VersionedValue{Raw: []byte("k1_value"), Version: ToBytes(35, 3)})
	assert.NoError(t, err)
	err = db.SetState(ns, "k111", driver.VersionedValue{Raw: []byte("k111_value"), Version: ToBytes(35, 4)})
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)
}

func TTestGetNonExistent(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"
	key := "foo"

	vv, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.VersionedValue{}, vv)
}

func TTestMetadata(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"
	key := "foo"

	md, ver, err := db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	assert.Nil(t, md)
	bn, txn, err := FromBytes(ver)
	assert.NoError(t, err)
	assert.Equal(t, uint64(0x0), bn)
	assert.Equal(t, uint64(0x0), txn)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetStateMetadata(ns, key, map[string][]byte{"foo": []byte("bar")}, ToBytes(35, 1))
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	md, ver, err = db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	bn, txn, err = FromBytes(ver)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"foo": []byte("bar")}, md)
	assert.Equal(t, uint64(35), bn)
	assert.Equal(t, uint64(1), txn)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetStateMetadata(ns, key, map[string][]byte{"foo1": []byte("bar1")}, ToBytes(36, 2))
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	md, ver, err = db.GetStateMetadata(ns, key)
	assert.NoError(t, err)
	bn, txn, err = FromBytes(ver)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"foo1": []byte("bar1")}, md)
	assert.Equal(t, uint64(36), bn)
	assert.Equal(t, uint64(2), txn)
}

func TTestDB1(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"
	key := "foo"
	keyWithSuffix := key + "/suffix"

	populateDB(t, db, ns, key, keyWithSuffix)

	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.DeleteState(ns, keyWithSuffix)
	assert.NoError(t, err)

	err = db.DeleteState(ns, key)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)
}

func TTestDB2(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"
	key := "foo"
	keyWithSuffix := key + "/suffix"

	populateDB(t, db, ns, key, keyWithSuffix)

	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.DeleteState(ns, key)
	assert.NoError(t, err)

	err = db.DeleteState(ns, keyWithSuffix)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)
}

func TTestRangeQueries1(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"

	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, "k2", driver.VersionedValue{Raw: []byte("k2_value"), Version: ToBytes(35, 1)})
	assert.NoError(t, err)
	err = db.SetState(ns, "k3", driver.VersionedValue{Raw: []byte("k3_value"), Version: ToBytes(35, 2)})
	assert.NoError(t, err)
	err = db.SetState(ns, "k1", driver.VersionedValue{Raw: []byte("k1_value"), Version: ToBytes(35, 3)})
	assert.NoError(t, err)
	err = db.SetState(ns, "k111", driver.VersionedValue{Raw: []byte("k111_value"), Version: ToBytes(35, 4)})
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	itr, err := db.GetStateRangeScanIterator(ns, "", "")
	assert.NoError(t, err)
	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: ToBytes(35, 1)},
		{Key: "k3", Raw: []byte("k3_value"), Version: ToBytes(35, 2)},
	}, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: ToBytes(35, 1)},
	}, res)
}

func TTestMultiWritesAndRangeQueries(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"
	assert.NoError(t, db.BeginUpdate())

	assert.NoError(t, db.SetState(ns, "k2", driver.VersionedValue{Raw: []byte("k2_value"), Version: ToBytes(35, 1)}))
	assert.NoError(t, db.SetState(ns, "k3", driver.VersionedValue{Raw: []byte("k3_value"), Version: ToBytes(35, 2)}))
	assert.NoError(t, db.SetState(ns, "k1", driver.VersionedValue{Raw: []byte("k1_value"), Version: ToBytes(35, 3)}))
	assert.NoError(t, db.SetState(ns, "k111", driver.VersionedValue{Raw: []byte("k111_value"), Version: ToBytes(35, 4)}))

	assert.NoError(t, db.Commit())

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		write(t, db, ns, "k2", []byte("k2_value"), 35, 1)
		wg.Done()
	}()
	go func() {
		write(t, db, ns, "k3", []byte("k3_value"), 35, 2)
		wg.Done()
	}()
	go func() {
		write(t, db, ns, "k1", []byte("k1_value"), 35, 3)
		wg.Done()
	}()
	go func() {
		write(t, db, ns, "k111", []byte("k111_value"), 35, 4)
		wg.Done()
	}()
	wg.Wait()

	itr, err := db.GetStateRangeScanIterator(ns, "", "")
	assert.NoError(t, err)
	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: ToBytes(35, 1)},
		{Key: "k3", Raw: []byte("k3_value"), Version: ToBytes(35, 2)},
	}, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	expected := []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: ToBytes(35, 1)},
	}
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadFirst(itr, 2)
	assert.NoError(t, err)
	expected = []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
	}
	assert.Len(t, res, 2)
	assert.Equal(t, expected, res)

	expected = []driver.VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: ToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: ToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: ToBytes(35, 1)},
	}
	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)
}

func TTestMultiWrites(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"
	var wg sync.WaitGroup
	n := 20
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			write(
				t,
				db,
				ns,
				fmt.Sprintf("TTestMultiWrites_key_%d", i),
				[]byte(fmt.Sprintf("TTestMultiWrites_value_%d", i)),
				35,
				1,
			)
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func write(t *testing.T, db driver.TransactionalVersionedPersistence, ns, key string, value []byte, block, txnum uint64) {
	tx, err := db.NewWriteTransaction()
	assert.NoError(t, err)
	err = tx.SetState(ns, key, driver.VersionedValue{Raw: value, Version: ToBytes(block, txnum)})
	assert.NoError(t, err)
	err = tx.Commit()
	assert.NoError(t, err)
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

func TTestCompositeKeys(t *testing.T, db driver.TransactionalVersionedPersistence) {
	ns := "namespace"
	keyPrefix := "prefix"

	err := db.BeginUpdate()
	assert.NoError(t, err)

	for _, comps := range [][]string{
		{"a", "b", "1"},
		{"a", "b"},
		{"a", "b", "3"},
		{"a", "d"},
	} {
		k, err := createCompositeKey(keyPrefix, comps)
		assert.NoError(t, err)
		err = db.SetState(ns, k, driver.VersionedValue{Raw: []byte(k), Version: ToBytes(35, 1)})
		assert.NoError(t, err)
	}

	err = db.Commit()
	assert.NoError(t, err)

	partialCompositeKey, err := createCompositeKey(keyPrefix, []string{"a"})
	assert.NoError(t, err)
	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	itr, err := db.GetStateRangeScanIterator(ns, startKey, endKey)
	assert.NoError(t, err)
	defer itr.Close()

	res := make([]driver.VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "\x00prefix0a0b0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30}, Version: ToBytes(0x23, 1)},
		{Key: "\x00prefix0a0b010", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x31, 0x30}, Version: ToBytes(0x23, 1)},
		{Key: "\x00prefix0a0b030", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x33, 0x30}, Version: ToBytes(0x23, 1)},
		{Key: "\x00prefix0a0d0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x64, 0x30}, Version: ToBytes(0x23, 1)},
	}, res)

	partialCompositeKey, err = createCompositeKey(keyPrefix, []string{"a", "b"})
	assert.NoError(t, err)
	startKey = partialCompositeKey
	endKey = partialCompositeKey + string(maxUnicodeRuneValue)

	itr, err = db.GetStateRangeScanIterator(ns, startKey, endKey)
	assert.NoError(t, err)
	defer itr.Close()

	res = make([]driver.VersionedRead, 0, 2)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.VersionedRead{
		{Key: "\x00prefix0a0b0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30}, Version: ToBytes(0x23, 1)},
		{Key: "\x00prefix0a0b010", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x31, 0x30}, Version: ToBytes(0x23, 1)},
		{Key: "\x00prefix0a0b030", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x33, 0x30}, Version: ToBytes(0x23, 1)},
	}, res)
}

// Postgres doesn't like non-utf8 in TEXT fields, so we made it a BYTEA.
// cannot check if key exists: pq: invalid byte sequence for encoding "UTF8": 0xc2 0x32]
func TTestNonUTF8keys(t *testing.T, db driver.UnversionedPersistence) {
	ns := "namespace"

	// adapted from https://www.php.net/manual/en/reference.pcre.pattern.modifiers.php#54805
	utf8 := map[string][]byte{
		"Invalid null":                                []byte("\x00"),
		"Invalid null included":                       []byte("\x00asdf"),
		"Valid ASCII":                                 []byte("a"),
		"Valid 2 Octet Sequence":                      []byte("\xc3\xb1"),
		"Invalid 2 Octet Sequence":                    []byte("\xc3\x28"),
		"Invalid Sequence Identifier":                 []byte("\xa0\xa1"),
		"Valid 3 Octet Sequence":                      []byte("\xe2\x82\xa1"),
		"Invalid 3 Octet Sequence (in 2nd Octet)":     []byte("\xe2\x28\xa1"),
		"Invalid 3 Octet Sequence (in 3rd Octet)":     []byte("\xe2\x82\x28"),
		"Valid 4 Octet Sequence":                      []byte("\xf0\x90\x8c\xbc"),
		"Invalid 4 Octet Sequence (in 2nd Octet)":     []byte("\xf0\x28\x8c\xbc"),
		"Invalid 4 Octet Sequence (in 3rd Octet)":     []byte("\xf0\x90\x28\xbc"),
		"Invalid 4 Octet Sequence (in 4th Octet)":     []byte("\xf0\x28\x8c\x28"),
		"Valid 5 Octet Sequence (but not Unicode!)":   []byte("\xf8\xa1\xa1\xa1\xa1"),
		"Valid 6 Octet Sequence (but not Unicode!)":   []byte("\xfc\xa1\xa1\xa1\xa1\xa1"),
		"Invalid byte sequence according to postgres": []byte("\xc2\032"),
		"Random bytes":                                {173, 88, 122, 110, 168, 232, 249, 57, 168, 242, 109, 36, 128, 116, 103, 188, 2, 145, 130, 160, 4, 171, 227, 82, 239, 58, 82, 94, 124, 4, 199, 143, 106, 27, 105, 144, 83, 7, 75, 50, 203, 167, 125, 40, 74, 4, 104, 4, 119, 192, 124, 36, 38, 152, 27, 68, 38, 59, 7, 70, 87, 157, 142, 207, 251, 4, 205, 184, 230, 66, 206, 56, 221, 223, 16, 141, 81, 222, 231, 47, 43, 135, 200, 74, 7, 220, 63, 128, 84, 60, 178, 67, 66, 82, 118, 141, 59, 38, 73, 220, 56, 178, 9, 249, 244, 6, 234, 162, 96, 66, 96, 95, 219, 179, 112, 172, 99, 4, 79, 226, 112, 203, 69, 149, 45, 214, 139, 147, 27, 209, 177, 118, 45, 226, 224, 161, 45, 228, 53, 114, 121, 127, 165, 115, 204, 60, 93, 41, 216, 115, 42, 230, 94, 15, 25, 51, 239, 245, 132, 56, 21, 214, 1, 7, 221, 56, 246, 134, 254, 178, 238, 162, 69, 103, 244, 241, 172, 32, 53, 250, 92, 20, 182, 235, 226, 16, 247, 96, 101, 165, 238, 115, 219, 168, 41, 143, 241, 126, 27, 186, 191, 114, 13, 99, 34, 12, 194, 107, 50, 128, 226, 5, 38, 228, 143, 250, 100, 26, 17, 215, 144, 79, 158, 78, 125, 228, 150, 226, 231, 84, 220, 237, 1, 239, 139, 86, 103, 150, 119, 219, 147, 56, 90, 149, 28, 100, 254, 129, 200, 58, 163, 227, 175, 48, 80, 218, 92, 180, 89, 102, 121, 242, 134, 68, 14, 171, 158, 66, 88, 124, 212, 179, 176, 2, 91, 56, 204, 80, 127, 192, 92, 129, 118, 176, 60, 102, 71, 250, 231, 10, 176, 39, 3, 10, 255, 169, 139, 89, 38, 137, 78, 219, 167, 118, 99, 151, 112, 168, 157, 138, 158, 155, 215, 15, 49, 3, 169, 51, 70, 81, 51, 9, 230, 180, 104, 159, 239, 72, 138, 183, 32, 189, 201, 111, 234, 41, 140, 29, 195, 98, 162, 105, 196, 171, 141, 62, 67, 119, 199, 100, 44, 243, 193, 136, 216, 213, 220, 181, 40, 222, 62, 44, 81, 45, 49, 35, 239, 96, 9, 255, 143, 3, 155, 179, 39, 38, 139, 82, 129, 235, 106, 109, 58, 188, 37, 114, 206, 202, 19, 239, 189, 249, 131, 255, 124, 246, 89, 176, 181, 43, 247, 173, 137, 7, 141, 252, 87, 233, 107, 123, 215, 158, 206, 81, 197, 63, 110, 10, 157, 113, 157, 152, 131, 148, 164, 115, 146, 112, 120, 69, 173, 103, 210, 173, 137, 61, 198, 242, 174, 150, 152, 83, 232, 215, 109, 110, 254, 227, 162, 183, 218, 211, 118, 103, 80, 44, 203, 187, 111, 23, 72, 31, 14, 152, 168, 199, 136, 230, 20, 66, 32, 11, 16, 63, 51, 0, 82, 109, 129, 70, 121, 142, 191, 252, 209, 109, 96, 234, 114, 156, 54, 145, 153, 202, 239, 120, 126, 150, 30, 159, 145, 233, 53, 137, 56, 108, 161, 71, 153, 6, 138, 167, 47, 37, 78, 125, 148, 18, 155, 58, 9, 217, 95, 142, 158, 211, 89, 245, 42, 129, 223, 64, 214, 170, 30, 143, 252, 138, 50, 34, 120, 71, 126, 42, 202, 3, 143, 72, 68, 217, 230, 41, 8, 3, 13, 97, 29, 111, 49, 31, 4, 148, 169, 240, 177, 19, 3, 93, 10, 145, 111, 65, 17, 80, 232, 191, 178, 250, 50, 241, 30, 41, 115, 170, 103, 79, 78, 196, 71, 113, 139, 228, 3, 211, 61, 141, 9, 224, 130, 244, 187, 9, 210, 129, 216, 218, 72, 77, 104, 153, 241, 38, 96, 85, 90, 190, 107, 186, 157, 53, 237, 102, 128, 146, 254, 31, 155, 119, 157, 96, 180, 91, 218, 255, 224, 159, 70, 166, 221, 60, 169, 245, 117, 36, 97, 33, 52, 94, 218, 201, 171, 209, 122, 8, 34, 205, 27, 239, 200, 124, 133, 112, 176, 187, 163, 30, 16, 137, 211, 49, 40, 153, 140, 119, 195, 64, 50, 236, 187, 238, 53, 108, 158, 141, 1, 76, 126, 109, 32, 119, 62, 73, 183, 94, 191, 125, 193, 79, 102, 20, 162, 199, 5, 153, 112, 4, 249, 5, 132, 36, 201, 219, 59, 17, 155, 234, 179, 90, 13, 16, 116, 243, 92, 63, 223, 122, 21, 209, 167, 240, 38, 112, 252, 220, 153, 148, 136, 96, 60, 39, 126, 126, 68, 234, 179, 104, 100, 55, 144, 192, 99, 52, 58, 80, 8, 176, 167, 187, 83, 135, 129, 188, 169, 34, 141, 203, 122, 176, 89, 63, 177, 186, 103, 128, 76, 236, 16, 62, 132, 203, 109, 120, 38, 106, 78, 117, 72, 203, 145, 178, 156, 115, 5, 53, 209, 210, 214, 119, 97, 251, 240, 83, 52, 239, 79, 57, 10, 145, 88, 215, 184, 202, 169, 188, 114, 182, 21, 176, 70, 221, 83, 4, 57, 245, 29, 141, 187, 96, 22, 45, 101, 65, 138, 117, 197, 48, 29, 212, 212, 134, 75, 91, 196, 31, 206, 79, 54, 48, 200, 103, 93, 66, 253, 28, 176, 0, 121, 161, 3, 34, 178, 0, 4, 196, 133, 50, 83, 129, 207, 181, 198, 135, 29, 72, 40, 211, 102, 81, 224, 52, 207, 68, 31, 133, 107, 204, 55, 136, 200, 93, 124, 156, 202, 92, 25, 114, 186, 251, 28, 60, 166, 201, 26, 152, 15, 230, 94, 117, 255, 75, 13, 137, 191, 250, 9, 78, 247, 155, 68, 149, 34, 144, 77, 100, 213, 55, 161, 186, 1, 118, 79, 252, 8, 64, 198, 149, 92, 15, 46, 144, 246, 116, 189, 128, 227, 83, 10, 103, 44, 41, 108, 165, 201, 166, 25, 153, 5, 247, 160, 3, 78, 55, 143, 247, 40, 131, 224, 246, 255, 103, 188, 229, 103, 72, 50, 236, 65, 196, 232, 8, 116, 203, 39, 57, 95, 10, 18, 33, 60, 192, 253, 15, 239, 26, 218, 166, 191, 233, 113, 228, 4, 77, 214, 136, 221, 154, 235, 75, 182, 10, 206, 19, 18, 79, 144, 146, 110, 218, 224, 68, 217, 47, 144, 128, 87, 32, 107, 133, 43, 53, 199, 158, 21, 46, 225, 167, 146, 36, 151, 36, 133, 50, 197, 233, 31, 53, 199, 205},
	}

	err := db.BeginUpdate()
	assert.NoError(t, err)
	for name, key := range utf8 {
		err = db.SetState(ns, string(key), key)
		assert.NoError(t, err, fmt.Sprintf("%s should be stored (%v)", name, key))
	}
	err = db.Commit()
	if err != nil {
		t.Fatal(err)
	}

	err = db.BeginUpdate()
	assert.NoError(t, err)
	for name, key := range utf8 {
		err = db.SetState(ns, string(key), key)
		assert.NoError(t, err, fmt.Sprintf("%s should be updated (%v)", name, key))
	}
	err = db.Commit()
	if err != nil {
		t.Fatal(err)
	}

	for name, key := range utf8 {
		v, err := db.GetState(ns, string(key))
		assert.NoError(t, err, fmt.Sprintf("%s should be retrieved (%v)", name, key))
		assert.Equal(t, key, v)
	}

	v, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)
}

var (
	namespace = "test_namespace"
	key       = "test_key"
)

func TTestUnversionedRange(t *testing.T, db driver.UnversionedPersistence) {
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
	assert.NoError(t, err)
	defer itr.Close()

	res := make([]driver.UnversionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
		{Key: "k3", Raw: []byte("k3_value")},
	}, res)

	itr, err = db.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	defer itr.Close()

	res = make([]driver.UnversionedRead, 0, 3)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}, res)

	itr, err = db.GetStateSetIterator(ns, "k1", "k2")
	assert.NoError(t, err)
	defer itr.Close()
	res = make([]driver.UnversionedRead, 0, 2)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 2)
	expected := []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}
	for _, read := range expected {
		assert.Contains(t, res, read)
	}
}

func TTestUnversionedSimple(t *testing.T, db driver.UnversionedPersistence) {
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

	v, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)

	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)

	// Discard an update
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(ns, key, []byte("val0"))
	assert.NoError(t, err)
	err = db.Discard()
	assert.NoError(t, err)

	// Expect state to be same as before the rollback
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

func BenchmarkConcatenation(b *testing.B) {
	var s string
	for i := 0; i < b.N; i++ {
		s = namespace + keys.NamespaceSeparator + key
	}
	_ = s
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
	_ = s
}

type notifyEvent struct {
	Op  opType
	NS  string
	Key string
}

func waitForResults[V any](ch <-chan V, times int, timeout time.Duration) ([]V, error) {
	results := make([]V, 0, times)

	for {
		select {
		case k := <-ch:
			results = append(results, k)
			if len(results) == times {
				return results, nil
			}
		case <-time.After(timeout):
			return nil, errors.New("timeout")
		}
	}
}

func TTestUnversionedNotifierSimple(t *testing.T, db driver.UnversionedNotifier) {
	ch, err := subscribe(db)
	assert.NoError(t, err)

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

	v, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)

	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)

	// Discard an update
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(ns, key, []byte("val0"))
	assert.NoError(t, err)
	err = db.Discard()
	assert.NoError(t, err)

	// Expect state to be same as before the rollback
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

	results, err := waitForResults(ch, 3, 1*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, []notifyEvent{{upsert, "ns", "key"}, {upsert, "ns", "key"}, {delete, "ns", "key"}}, results)
}

func TTestVersionedNotifierSimple(t *testing.T, db driver.VersionedNotifier) {
	ch, err := subscribe(db)
	assert.NoError(t, err)

	ns := "ns"
	key := "key"

	vv, err := db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), vv.Raw)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(ns, key, driver.VersionedValue{Raw: []byte("val")})
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), vv.Raw)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(ns, key, driver.VersionedValue{Raw: []byte("val1")})
	assert.NoError(t, err)

	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), vv.Raw)

	err = db.Commit()
	assert.NoError(t, err)

	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), vv.Raw)

	// Discard an update
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(ns, key, driver.VersionedValue{Raw: []byte("val0")})
	assert.NoError(t, err)
	err = db.Discard()
	assert.NoError(t, err)

	// Expect state to be same as before the rollback
	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), vv.Raw)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.DeleteState(ns, key)
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	vv, err = db.GetState(ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), vv.Raw)

	results, err := waitForResults(ch, 3, 1*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, []notifyEvent{{upsert, "ns", "key"}, {upsert, "ns", "key"}, {delete, "ns", "key"}}, results)
}

type opType int

const (
	unknown opType = iota
	delete
	upsert
)

// We treat update/inserts as the same, because we don't need the operation type.
// Distinguishing the two cases for sqlite would require more logic.
var opTypeMap = map[driver.Operation]opType{
	driver.Unknown: unknown,
	driver.Update:  upsert,
	driver.Insert:  upsert,
	driver.Delete:  delete,
}

type notifier interface {
	Subscribe(callback driver.TriggerCallback) error
}

func subscribe(db notifier) (chan notifyEvent, error) {
	ch := make(chan notifyEvent, 100) //TODO: AF Why deadlock
	err := db.Subscribe(func(operation driver.Operation, m map[driver.ColumnKey]string) {
		ch <- notifyEvent{Op: opTypeMap[operation], NS: m["ns"], Key: m["pkey"]}
	})
	if err != nil {
		return nil, err
	}
	time.Sleep(1 * time.Second) // Wait until subscription is complete before inserting values
	return ch, nil
}

func ToBytes(Block driver2.BlockNum, TxNum driver2.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(Block))
	binary.BigEndian.PutUint32(buf[4:], uint32(TxNum))
	return buf
}

func FromBytes(data driver3.RawVersion) (driver2.BlockNum, driver2.TxNum, error) {
	if len(data) == 0 {
		return 0, 0, nil
	}
	if len(data) != 8 {
		return 0, 0, errors.Errorf("block number must be 8 bytes, but got %d", len(data))
	}
	Block := driver2.BlockNum(binary.BigEndian.Uint32(data[:4]))
	TxNum := driver2.TxNum(binary.BigEndian.Uint32(data[4:]))
	return Block, TxNum, nil
}
