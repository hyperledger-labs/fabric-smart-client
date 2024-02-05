/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package badger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dgraph-io/badger/v3"
	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger/mock"
	dbproto "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	var err error
	tempDir, err = os.MkdirTemp("", "badger-fsc-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temporary directory: %v", err)
		os.Exit(-1)
	}
	defer os.RemoveAll(tempDir)

	m.Run()
}

func TestDriverImpl(t *testing.T) {
	tempDir := t.TempDir()
	for _, c := range dbtest.Cases {
		db := initBadger(t, tempDir, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		db := initBadger(t, tempDir, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, &unversioned.Unversioned{Versioned: db})
		})
	}
}

func initBadger(t *testing.T, tempDir, key string) driver.TransactionalVersionedPersistence {
	dbpath := filepath.Join(tempDir, key)
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if db == nil {
		t.Fatal("database is nil")
	}

	return db
}

func marshalOrPanic(o proto.Message) []byte {
	data, err := proto.Marshal(o)
	if err != nil {
		panic(err)
	}
	return data
}

var tempDir string

func TestMarshallingErrors(t *testing.T) {
	ns := "ns"
	key := "key"

	dbpath := filepath.Join(tempDir, "DB-TestMarshallingErrors")
	db, err := OpenDB(Opts{Path: dbpath}, nil)
	assert.NoError(t, err)
	defer db.Close()
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
