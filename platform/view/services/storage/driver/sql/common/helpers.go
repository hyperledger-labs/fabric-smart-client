/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/stretchr/testify/assert"
)

// This file exposes functions that db drivers can use for integration tests

var Cases = []struct {
	Name string
	Fn   func(*testing.T, driver.KeyValueStore)
}{
	{"RangeQueries", TTestRangeQueries},
	{"SimpleReadWrite", TTestSimpleReadWrite},
	{"GetNonExistent", TTestGetNonExistent},
	{"DB1", TTestDB1},
	{"DB2", TTestDB2},
	{"RangeQueries1", TTestRangeQueries1},
	{"MultiWritesAndRangeQueries", TTestMultiWritesAndRangeQueries},
	{"MultiWrites", TTestMultiWrites},
	{"CompositeKeys", TTestCompositeKeys},
}

var UnversionedCases = []struct {
	Name string
	Fn   func(*testing.T, driver.KeyValueStore)
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

var ErrorCases = []struct {
	Name string
	Fn   func(t *testing.T, readDB *sql.DB, writeDB WriteDB, errorWrapper driver.SQLErrorWrapper, table string)
}{
	{"Duplicate", TTestDuplicate},
}

func TTestDuplicate(t *testing.T, _ *sql.DB, writeDB WriteDB, errorWrapper driver.SQLErrorWrapper, table string) {
	ns := "namespace"

	tx, err := writeDB.Begin()
	assert.NoError(t, err, "should start tx")

	query := fmt.Sprintf("INSERT INTO %s (ns, pkey, val) VALUES ($1, $2, $3)", table)

	_, err = tx.Exec(query, ns, "key", []byte("test 1"))
	assert.NoError(t, err, "should insert first row")

	_, err = tx.Exec(query, ns, "key", []byte("test 2"))
	assert.Error(t, err, "should fail on duplicate")
	assert.True(t, errors.HasCause(errorWrapper.WrapError(err), driver.UniqueKeyViolation), "should be a unique-key violation")

	err = tx.Rollback()
	assert.NoError(t, err, "should rollback")
}

func TTestRangeQueries(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	entries := map[string]driver.UnversionedValue{
		"k1":   driver.UnversionedValue("k1_value"),
		"k111": driver.UnversionedValue("k111_value"),
		"k2":   driver.UnversionedValue("k2_value"),
		"k3":   driver.UnversionedValue("k3_value"),
	}
	populateDB(t, db, ns, entries)
	defer cleanupDB(t, db, ns)

	itr, err := db.GetStateRangeScanIterator(context.Background(), ns, "", "")
	assert.NoError(t, err)
	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
		{Key: "k3", Raw: []byte("k3_value")},
	}, res)

	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	expected := []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)

	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadFirst(itr, 2)
	assert.NoError(t, err)
	expected = []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
	}
	assert.Len(t, res, 2)
	assert.Equal(t, expected, res)

	expected = []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}
	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)

	expected = []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k3", Raw: []byte("k3_value")},
	}
	itr, err = db.GetStateSetIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 2)
	for _, read := range expected {
		assert.Contains(t, res, read)
	}
}

func TTestSimpleReadWrite(t *testing.T, db driver.KeyValueStore) {
	ns := "ns"
	key := "key"
	defer cleanupDB(t, db, ns)

	// empty state
	vv, err := db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Nil(t, vv)

	// add data
	err = db.SetState(context.Background(), ns, key, driver.UnversionedValue("val"))
	assert.NoError(t, err)

	// get data
	vv, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.UnversionedValue("val"), vv)

	// logging because this can cause a deadlock if maxOpenConnections is only 1
	t.Logf("get state [%s] during set state tx", key)
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, key, driver.UnversionedValue("val1"))
	assert.NoError(t, err)

	vv, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.UnversionedValue("val"), vv)
	err = db.Commit()
	assert.NoError(t, err)

	t.Logf("get state after tx [%s]", key)
	vv, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.UnversionedValue("val1"), vv)

	// Discard an update
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, key, driver.UnversionedValue("val0"))
	assert.NoError(t, err)
	err = db.Discard()
	assert.NoError(t, err)

	// Expect state to be same as before the rollback
	vv, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, driver.UnversionedValue("val1"), vv)

	// deleteOp state
	err = db.DeleteState(context.Background(), ns, key)
	assert.NoError(t, err)

	// expect state to be empty
	vv, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Nil(t, vv)
}

func populateDB(t *testing.T, db driver.KeyValueStore, ns string, entries map[string]driver.UnversionedValue) {
	assert.NoError(t, db.BeginUpdate())
	assert.Empty(t, db.SetStates(t.Context(), ns, entries))
	assert.NoError(t, db.Commit())

	// let's check that we only have
	itr, err := db.GetStateRangeScanIterator(t.Context(), ns, "", "")
	assert.NoError(t, err)

	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Equal(t, len(entries), len(res))

	for _, r := range res {
		v, ok := entries[r.Key]
		assert.True(t, ok)
		assert.Equal(t, v, r.Raw)
	}
}

func cleanupDB(t *testing.T, db driver.KeyValueStore, ns string) {
	assert.NoError(t, db.BeginUpdate())
	itr, err := db.GetStateRangeScanIterator(t.Context(), ns, "", "")
	assert.NoError(t, err)

	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)

	var keys []string
	for _, r := range res {
		keys = append(keys, r.Key)
	}

	errs := db.DeleteStates(t.Context(), ns, keys...)
	assert.Empty(t, errs)
	assert.NoError(t, db.Commit())
}

func TTestGetNonExistent(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	key := "foo"

	vv, err := db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Nil(t, vv)
}

func TTestDB1(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	key := "foo"
	keyWithSuffix := key + "/suffix"

	entries := map[string]driver.UnversionedValue{
		key:           driver.UnversionedValue("bar"),
		keyWithSuffix: driver.UnversionedValue("bar1"),
	}
	populateDB(t, db, ns, entries)

	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.DeleteState(context.Background(), ns, keyWithSuffix)
	assert.NoError(t, err)

	err = db.DeleteState(context.Background(), ns, key)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)
}

func TTestDB2(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	key := "foo"
	keyWithSuffix := key + "/suffix"

	entries := map[string]driver.UnversionedValue{
		key:           driver.UnversionedValue("bar"),
		keyWithSuffix: driver.UnversionedValue("bar1"),
	}
	populateDB(t, db, ns, entries)

	err := db.BeginUpdate()
	assert.NoError(t, err)

	err = db.DeleteState(context.Background(), ns, key)
	assert.NoError(t, err)

	err = db.DeleteState(context.Background(), ns, keyWithSuffix)
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)
}

func TTestRangeQueries1(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	entries := map[string]driver.UnversionedValue{
		"k2":   driver.UnversionedValue("k2_value"),
		"k3":   driver.UnversionedValue("k3_value"),
		"k1":   driver.UnversionedValue("k1_value"),
		"k111": driver.UnversionedValue("k111_value"),
	}
	populateDB(t, db, ns, entries)
	defer cleanupDB(t, db, ns)

	itr, err := db.GetStateRangeScanIterator(context.Background(), ns, "", "")
	assert.NoError(t, err)
	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
		{Key: "k3", Raw: []byte("k3_value")},
	}, res)

	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}, res)
}

func TTestMultiWritesAndRangeQueries(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	entries := map[string]driver.UnversionedValue{
		"k1":   driver.UnversionedValue("k1_value"),
		"k2":   driver.UnversionedValue("k2_value"),
		"k3":   driver.UnversionedValue("k3_value"),
		"k111": driver.UnversionedValue("k111_value"),
	}
	populateDB(t, db, ns, entries)
	defer cleanupDB(t, db, ns)

	var wg sync.WaitGroup
	for k, v := range entries {
		wg.Add(1)
		go func(k string, v driver.UnversionedValue) {
			defer wg.Done()
			assert.NoError(t, db.SetState(context.Background(), ns, k, v))
		}(k, v)
	}
	wg.Wait()

	itr, err := db.GetStateRangeScanIterator(context.Background(), ns, "", "")
	assert.NoError(t, err)
	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 4)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
		{Key: "k3", Raw: []byte("k3_value")},
	}, res)

	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	expected := []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)

	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadFirst(itr, 2)
	assert.NoError(t, err)
	expected = []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
	}
	assert.Len(t, res, 2)
	assert.Equal(t, expected, res)

	expected = []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}
	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.Equal(t, expected, res)
}

func TTestMultiWrites(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	key := "test_key"
	defer cleanupDB(t, db, ns)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			assert.NoError(t, db.SetState(context.Background(), ns, key, []byte(fmt.Sprintf("TTestMultiWrites_value_%d", i))))
		}(i)
	}
	wg.Wait()
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

func TTestCompositeKeys(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	keyPrefix := "prefix"
	defer cleanupDB(t, db, ns)

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
		err = db.SetState(context.Background(), ns, k, driver.UnversionedValue(k))
		assert.NoError(t, err)
	}

	err = db.Commit()
	assert.NoError(t, err)

	partialCompositeKey, err := createCompositeKey(keyPrefix, []string{"a"})
	assert.NoError(t, err)
	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	itr, err := db.GetStateRangeScanIterator(context.Background(), ns, startKey, endKey)
	assert.NoError(t, err)

	res, err := iterators.ReadAllValues(itr)
	assert.NoError(t, err)

	assert.Len(t, res, 4)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "\x00prefix0a0b0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30}},
		{Key: "\x00prefix0a0b010", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x31, 0x30}},
		{Key: "\x00prefix0a0b030", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x33, 0x30}},
		{Key: "\x00prefix0a0d0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x64, 0x30}},
	}, res)

	partialCompositeKey, err = createCompositeKey(keyPrefix, []string{"a", "b"})
	assert.NoError(t, err)
	startKey = partialCompositeKey
	endKey = partialCompositeKey + string(maxUnicodeRuneValue)

	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, startKey, endKey)
	assert.NoError(t, err)

	res, err = iterators.ReadAllValues(itr)
	assert.NoError(t, err)

	assert.Len(t, res, 3)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "\x00prefix0a0b0", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30}},
		{Key: "\x00prefix0a0b010", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x31, 0x30}},
		{Key: "\x00prefix0a0b030", Raw: []uint8{0x0, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x30, 0x61, 0x30, 0x62, 0x30, 0x33, 0x30}},
	}, res)
}

// Postgres doesn't like non-utf8 in TEXT fields, so we made it a BYTEA.
// cannot check if key exists: pq: invalid byte sequence for encoding "UTF8": 0xc2 0x32]
func TTestNonUTF8keys(t *testing.T, db driver.KeyValueStore) {
	ns := "namespace"
	key := "test_key"
	defer cleanupDB(t, db, ns)

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
		err = db.SetState(context.Background(), ns, string(key), key)
		assert.NoError(t, err, fmt.Sprintf("%s should be stored (%v)", name, key))
	}
	err = db.Commit()
	if err != nil {
		t.Fatal(err)
	}

	err = db.BeginUpdate()
	assert.NoError(t, err)
	for name, key := range utf8 {
		err = db.SetState(context.Background(), ns, string(key), key)
		assert.NoError(t, err, fmt.Sprintf("%s should be updated (%v)", name, key))
	}
	err = db.Commit()
	if err != nil {
		t.Fatal(err)
	}

	for name, key := range utf8 {
		v, err := db.GetState(context.Background(), ns, string(key))
		assert.NoError(t, err, fmt.Sprintf("%s should be retrieved (%v)", name, key))
		assert.Equal(t, key, v)
	}

	v, err := db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Nil(t, v)
}

func TTestUnversionedRange(t *testing.T, db driver.KeyValueStore) {
	var err error

	ns := "namespace"
	defer cleanupDB(t, db, ns)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(context.Background(), ns, "k2", []byte("k2_value"))
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, "k3", []byte("k3_value"))
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, "k1", []byte("k1_value"))
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, "k111", []byte("k111_value"))
	assert.NoError(t, err)

	err = db.Commit()
	assert.NoError(t, err)

	itr, err := db.GetStateRangeScanIterator(context.Background(), ns, "", "")
	assert.NoError(t, err)

	res, err := iterators.ReadAllValues(itr)
	assert.NoError(t, err)

	assert.Len(t, res, 4)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
		{Key: "k3", Raw: []byte("k3_value")},
	}, res)

	itr, err = db.GetStateRangeScanIterator(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)

	res, err = iterators.ReadAllValues(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k111", Raw: []byte("k111_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}, res)

	itr, err = db.GetStateSetIterator(context.Background(), ns, "k1", "k2")
	assert.NoError(t, err)

	res, err = iterators.ReadAllValues(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 2)
	expected := []driver.UnversionedRead{
		{Key: "k1", Raw: []byte("k1_value")},
		{Key: "k2", Raw: []byte("k2_value")},
	}
	for _, read := range expected {
		assert.Contains(t, res, read)
	}
}

func TTestUnversionedSimple(t *testing.T, db driver.KeyValueStore) {
	ns := "ns"
	key := "key"
	defer cleanupDB(t, db, ns)

	v, err := db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, key, []byte("val"))
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(context.Background(), ns, key, []byte("val1"))
	assert.NoError(t, err)

	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)

	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)

	// Discard an update
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, key, []byte("val0"))
	assert.NoError(t, err)
	err = db.Discard()
	assert.NoError(t, err)

	// Expect state to be same as before the rollback
	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.DeleteState(context.Background(), ns, key)
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)
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
	defer cleanupDB(t, db, ns)

	v, err := db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, key, []byte("val"))
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)

	err = db.SetState(context.Background(), ns, key, []byte("val1"))
	assert.NoError(t, err)

	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), v)

	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)

	// Discard an update
	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.SetState(context.Background(), ns, key, []byte("val0"))
	assert.NoError(t, err)
	err = db.Discard()
	assert.NoError(t, err)

	// Expect state to be same as before the rollback
	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val1"), v)

	err = db.BeginUpdate()
	assert.NoError(t, err)
	err = db.DeleteState(context.Background(), ns, key)
	assert.NoError(t, err)
	err = db.Commit()
	assert.NoError(t, err)

	v, err = db.GetState(context.Background(), ns, key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)

	results, err := waitForResults(ch, 3, 1*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, []notifyEvent{{upsertOp, "ns", "key"}, {upsertOp, "ns", "key"}, {deleteOp, "ns", "key"}}, results)
}

type opType int

const (
	unknownOp opType = iota
	deleteOp
	upsertOp
)

// We treat update/inserts as the same, because we don't need the operation type.
// Distinguishing the two cases for sqlite would require more logic.
var opTypeMap = map[driver.Operation]opType{
	driver.Unknown: unknownOp,
	driver.Update:  upsertOp,
	driver.Insert:  upsertOp,
	driver.Delete:  deleteOp,
}

type notifier interface {
	Subscribe(callback driver.TriggerCallback) error
}

func subscribe(db notifier) (chan notifyEvent, error) {
	ch := make(chan notifyEvent, 100) // TODO: AF Why deadlock
	err := db.Subscribe(func(operation driver.Operation, m map[driver.ColumnKey]string) {
		ch <- notifyEvent{Op: opTypeMap[operation], NS: m["ns"], Key: m["pkey"]}
	})
	if err != nil {
		return nil, err
	}
	time.Sleep(1 * time.Second) // Wait until subscription is complete before inserting values
	return ch, nil
}
