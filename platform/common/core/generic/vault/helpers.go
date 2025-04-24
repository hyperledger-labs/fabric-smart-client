/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
)

type ValidationCode = int

const (
	_ ValidationCode = iota
	valid
	invalid
	busy
	unknown
)

var RemoveNils func(items []driver.VaultRead) []driver.VaultRead

var VCProvider = driver.NewValidationCodeProvider(map[ValidationCode]driver.TxStatusCode{
	valid:   driver.Valid,
	invalid: driver.Invalid,
	busy:    driver.Busy,
	unknown: driver.Unknown,
})

type artifactsProvider interface {
	NewCachedVault(ddb driver2.VaultStore) (*Vault[ValidationCode], error)
	NewNonCachedVault(ddb driver2.VaultStore) (*Vault[ValidationCode], error)
	NewMarshaller() Marshaller
}

var SingleDBCases = []struct {
	Name string
	Fn   func(*testing.T, driver2.VaultStore, artifactsProvider)
}{
	{"Merge", TTestMerge},
	{"Inspector", TTestInspector},
	{"InterceptorErr", TTestInterceptorErr},
	{"InterceptorConcurrency", TTestInterceptorConcurrency},
	{"QueryExecutor", TTestQueryExecutor},
	{"ShardLikeCommit", TTestShardLikeCommit},
	{"VaultErr", TTestVaultErr},
}

var ReadCommittedDBCases = []struct {
	Name string
	Fn   func(*testing.T, driver2.VaultStore, artifactsProvider)
}{
	{"InterceptorConcurrencyNoConsistency", TTestInterceptorConcurrencyNoConsistency},
}

var DoubleDBCases = []struct {
	Name string
	Fn   func(*testing.T, driver2.VaultStore, driver2.VaultStore, artifactsProvider)
}{
	{"Run", TTestRun},
}

func TTestInterceptorErr(t *testing.T, ddb driver2.VaultStore, vp artifactsProvider) {
	vault1, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	rws, err := vault1.NewRWSet(context.Background(), "txid")
	assert.NoError(t, err)

	_, err = rws.GetState("foo", "bar", 15)
	assert.EqualError(t, err, "invalid get option [15]")
	_, err = rws.GetState("foo", "bar", 15, 16)
	assert.EqualError(t, err, "a single getoption is supported, 2 provided")

	_, err = rws.GetStateMetadata("foo", "bar", 15)
	assert.EqualError(t, err, "invalid get option [15]")
	_, err = rws.GetStateMetadata("foo", "bar", 15, 16)
	assert.EqualError(t, err, "a single getoption is supported, 2 provided")

	rws.Done()

	_, err = rws.GetStateMetadata("foo", "bar")
	assert.EqualError(t, err, "this instance was closed")
	_, err = rws.GetState("foo", "bar")
	assert.EqualError(t, err, "this instance was closed")
	err = rws.SetState("foo", "bar", []byte("whocares"))
	assert.EqualError(t, err, "this instance was closed")
	err = rws.SetStateMetadata("foo", "bar", nil)
	assert.EqualError(t, err, "this instance was closed")
	err = rws.DeleteState("foo", "bar")
	assert.EqualError(t, err, "this instance was closed")
	_, _, err = rws.GetReadAt("foo", 12312)
	assert.EqualError(t, err, "this instance was closed")
	_, _, err = rws.GetWriteAt("foo", 12312)
	assert.EqualError(t, err, "this instance was closed")
	err = rws.AppendRWSet([]byte("foo"))
	assert.EqualError(t, err, "this instance was closed")

	rws.Done()
	rws, err = vault1.NewRWSet(context.Background(), "validtxid")
	assert.NoError(t, err)
	rws.Done()
	err = vault1.CommitTX(context.TODO(), "validtxid", 2, 3)
	assert.NoError(t, err)
	rws, err = vault1.NewRWSet(context.Background(), "validtxid")
	assert.NoError(t, err)
	rws.Done()
}

func TTestInterceptorConcurrency(t *testing.T, ddb driver2.VaultStore, vp artifactsProvider) {
	ns := "namespace"
	k := "key1"

	vault1, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	rws, err := vault1.NewRWSet(context.Background(), "txid")
	assert.NoError(t, err)

	v, err := rws.GetState(ns, k)
	assert.NoError(t, err)
	assert.Nil(t, v)

	go func() {
		time.Sleep(2 * time.Second)
		rws.Done()
	}()

	err = ddb.Store(context.Background(), nil, driver.Writes{
		ns: map[driver.PKey]driver.VaultValue{
			k: {Raw: []byte("val"), Version: versionBlockTxNumToBytes(35, 1)},
		},
	}, nil)
	assert.NoError(t, err)

	state, err := ddb.GetState(context.Background(), ns, k)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val"), state.Raw)
}

func TTestInterceptorConcurrencyNoConsistency(t *testing.T, ddb driver2.VaultStore, vp artifactsProvider) {
	ns := "namespace"
	k := "key1"
	mk := "meyakey1"

	vault1, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	rws, err := vault1.NewRWSetWithIsolationLevel(context.Background(), "txid", driver.LevelReadUncommitted)
	assert.NoError(t, err)

	v, err := rws.GetState(ns, k)
	assert.NoError(t, err)
	assert.Nil(t, v)

	err = ddb.Store(context.Background(), nil, driver.Writes{
		ns: map[driver.PKey]driver.VaultValue{
			k: {Raw: []byte("val"), Version: versionBlockTxNumToBytes(35, 1)},
		},
	}, nil)
	assert.NoError(t, err)

	_, _, err = rws.GetReadAt(ns, 0)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at version [[]], current value at version [[0 0 0 35 0 0 0 1]]")

	_, err = rws.GetState(ns, k)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at version [[]], current value at version [[0 0 0 35 0 0 0 1]]")

	mv, err := rws.GetStateMetadata(ns, mk)
	assert.NoError(t, err)
	assert.Nil(t, mv)

	err = ddb.Store(context.Background(), nil, nil, driver.MetaWrites{
		ns: map[driver.PKey]driver.VaultMetadataValue{
			mk: {
				Version:  versionBlockTxNumToBytes(36, 1),
				Metadata: map[string][]byte{"k": []byte("v")},
			},
		},
	})
	assert.NoError(t, err)

	_, err = rws.GetStateMetadata(ns, mk)
	assert.EqualError(t, err, "invalid metadata read: previous value returned at version [[]], current value at version [[0 0 0 36 0 0 0 1]]")
}

func TTestQueryExecutor(t *testing.T, ddb driver2.VaultStore, vp artifactsProvider) {
	ns := "namespace"

	aVault, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)

	err = ddb.Store(context.Background(), nil, driver.Writes{
		ns: map[driver.PKey]driver.VaultValue{
			"k2":   {Raw: []byte("k2_value"), Version: versionBlockTxNumToBytes(35, 1)},
			"k3":   {Raw: []byte("k3_value"), Version: versionBlockTxNumToBytes(35, 2)},
			"k1":   {Raw: []byte("k1_value"), Version: versionBlockTxNumToBytes(35, 3)},
			"k111": {Raw: []byte("k111_value"), Version: versionBlockTxNumToBytes(35, 4)},
		},
	}, nil)
	assert.NoError(t, err)

	qe, err := aVault.NewQueryExecutor(context.Background())
	assert.NoError(t, err)

	v, err := qe.GetState(context.Background(), ns, "k1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("k1_value"), v.Raw)
	v, err = qe.GetState(context.Background(), ns, "barfobarfs")
	assert.NoError(t, err)
	assert.Nil(t, v)

	itr, err := qe.GetStateRange(context.Background(), ns, "", "")
	assert.NoError(t, err)
	res, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 4)
	assert.ElementsMatch(t, []driver.VaultRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: versionBlockTxNumToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: versionBlockTxNumToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: versionBlockTxNumToBytes(35, 1)},
		{Key: "k3", Raw: []byte("k3_value"), Version: versionBlockTxNumToBytes(35, 2)},
	}, res)

	itr, err = ddb.GetStateRange(context.Background(), ns, "k1", "k3")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.Equal(t, []driver.VaultRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: versionBlockTxNumToBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: versionBlockTxNumToBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: versionBlockTxNumToBytes(35, 1)},
	}, res)

	itr, err = ddb.GetStates(context.Background(), ns, "k1", "k2", "k111")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Len(t, res, 3)
	assert.ElementsMatch(t, []driver.VaultRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: versionBlockTxNumToBytes(35, 3)},
		{Key: "k2", Raw: []byte("k2_value"), Version: versionBlockTxNumToBytes(35, 1)},
		{Key: "k111", Raw: []byte("k111_value"), Version: versionBlockTxNumToBytes(35, 4)},
	}, res)

	itr, err = ddb.GetStates(context.Background(), ns, "k1", "k5")
	assert.NoError(t, err)
	res, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	var expected = RemoveNils([]driver.VaultRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: versionBlockTxNumToBytes(35, 3)},
	})
	assert.Equal(t, expected, res)
}

func TTestShardLikeCommit(t *testing.T, ddb driver2.VaultStore, vp artifactsProvider) {
	ns := "namespace"
	k1 := "key1"
	k2 := "key2"

	// Populate the DB with some data at some height
	err := ddb.Store(context.Background(), nil, driver.Writes{
		ns: map[driver.PKey]driver.VaultValue{
			k1: {Raw: []byte("k1val"), Version: versionBlockTxNumToBytes(35, 1)},
			k2: {Raw: []byte("k2val"), Version: versionBlockTxNumToBytes(37, 3)},
		},
	}, nil)
	assert.NoError(t, err)

	aVault, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)

	// SCENARIO 1: there is a read conflict in the proposed rwset
	// create the read-write set
	rwsb := &ReadWriteSet{
		ReadSet: ReadSet{
			Reads:        map[driver.Namespace]NamespaceReads{},
			OrderedReads: map[string][]string{},
		},
		WriteSet: WriteSet{
			Writes:        map[string]NamespaceWrites{},
			OrderedWrites: map[string][]string{},
		},
		MetaWriteSet: MetaWriteSet{
			MetaWrites: map[string]KeyedMetaWrites{},
		},
	}
	rwsb.ReadSet.Add(ns, k1, versionBlockTxNumToBytes(35, 1))
	rwsb.ReadSet.Add(ns, k2, versionBlockTxNumToBytes(37, 2))
	assert.NoError(t, rwsb.WriteSet.Add(ns, k1, []byte("k1FromTxidInvalid")))
	assert.NoError(t, rwsb.WriteSet.Add(ns, k2, []byte("k2FromTxidInvalid")))
	marshaller := vp.NewMarshaller()
	rwsBytes, err := marshaller.Marshal("pineapple", rwsb)
	assert.NoError(t, err)

	// give it to the kvs and check whether it's valid - it won't be
	rwset, err := aVault.NewRWSetFromBytes(context.Background(), "txid-invalid", rwsBytes)
	assert.NoError(t, err)
	err = rwset.IsValid()
	assert.EqualError(t, err, "invalid read: vault at version namespace:key2 [&{key2 [107 50 118 97 108] [0 0 0 37 0 0 0 3]}], read-write set at version [[0 0 0 37 0 0 0 2]]")

	// close the read-write set, even in case of error
	rwset.Done()

	// check the status, it should be busy
	code, _, err := aVault.Status(context.Background(), "txid-invalid")
	assert.NoError(t, err)
	assert.Equal(t, busy, code)

	// now in case of error we won't commit the read-write set, so we should discard it
	err = aVault.DiscardTx(context.Background(), "txid-invalid", "")
	assert.NoError(t, err)

	// check the status, it should be invalid
	code, _, err = aVault.Status(context.Background(), "txid-invalid")
	assert.NoError(t, err)
	assert.Equal(t, invalid, code)

	// SCENARIO 2: there is no read conflict
	// create the read-write set
	rwsb = &ReadWriteSet{
		ReadSet: ReadSet{
			Reads:        map[driver.Namespace]NamespaceReads{},
			OrderedReads: map[string][]string{},
		},
		WriteSet: WriteSet{
			Writes:        map[string]NamespaceWrites{},
			OrderedWrites: map[string][]string{},
		},
		MetaWriteSet: MetaWriteSet{
			MetaWrites: map[string]KeyedMetaWrites{},
		},
	}
	rwsb.ReadSet.Add(ns, k1, versionBlockTxNumToBytes(35, 1))
	rwsb.ReadSet.Add(ns, k2, versionBlockTxNumToBytes(37, 3))
	assert.NoError(t, rwsb.WriteSet.Add(ns, k1, []byte("k1FromTxidValid")))
	assert.NoError(t, rwsb.WriteSet.Add(ns, k2, []byte("k2FromTxidValid")))
	rwsBytes, err = marshaller.Marshal("pineapple", rwsb)
	assert.NoError(t, err)

	// give it to the kvs and check whether it's valid - it will be
	rwset, err = aVault.NewRWSetFromBytes(context.Background(), "txid-valid", rwsBytes)
	assert.NoError(t, err)
	err = rwset.IsValid()
	assert.NoError(t, err)

	// close the read-write set
	rwset.Done()

	// presumably the cross-shard protocol continues...

	// check the status, it should be busy
	code, _, err = aVault.Status(context.Background(), "txid-valid")
	assert.NoError(t, err)
	assert.Equal(t, busy, code)

	// we're now asked to really commit
	err = aVault.CommitTX(context.TODO(), "txid-valid", 38, 10)
	assert.NoError(t, err)

	// check the status, it should be valid
	code, _, err = aVault.Status(context.Background(), "txid-valid")
	assert.NoError(t, err)
	assert.Equal(t, valid, code)

	// check the content of the kvs after that
	vv, err := ddb.GetState(context.Background(), ns, k1)
	assert.NoError(t, err)
	assert.Equal(t, &driver.VaultRead{Key: "key1", Raw: []byte("k1FromTxidValid"), Version: versionBlockTxNumToBytes(38, 10)}, vv)

	vv, err = ddb.GetState(context.Background(), ns, k2)
	assert.NoError(t, err)
	assert.Equal(t, &driver.VaultRead{Key: "key2", Raw: []byte("k2FromTxidValid"), Version: versionBlockTxNumToBytes(38, 10)}, vv)

	// all interceptors should be gone
	assert.Len(t, aVault.interceptors, 0)
}

func TTestVaultErr(t *testing.T, ddb driver2.VaultStore, vp artifactsProvider) {
	vault1, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	err = vault1.CommitTX(context.TODO(), "non-existent", 0, 0)
	assert.ErrorContains(t, err, "read-write set for txids [[non-existent]] could not be found")
	err = vault1.DiscardTx(context.Background(), "non-existent", "")
	assert.EqualError(t, err, "read-write set for txids [[non-existent]] could not be found")

	rws := &ReadWriteSet{
		ReadSet: ReadSet{
			Reads:        map[driver.Namespace]NamespaceReads{},
			OrderedReads: map[string][]string{},
		},
	}
	rws.ReadSet.Add("pineapple", "key", versionBlockTxNumToBytes(35, 1))
	m := vp.NewMarshaller()
	rwsBytes, err := m.Marshal("pineapple", rws)
	assert.NoError(t, err)

	ncrwset, err := vault1.NewRWSet(context.Background(), "not-closed")
	assert.NoError(t, err)
	_, err = vault1.NewRWSet(context.Background(), "not-closed")
	assert.EqualError(t, err, "programming error: rwset already exists for [not-closed]")
	_, err = vault1.NewRWSetFromBytes(context.Background(), "not-closed", rwsBytes)
	assert.ErrorContains(t, err, "programming error: rwset already exists for [not-closed]")
	err = vault1.CommitTX(context.TODO(), "not-closed", 0, 0)
	assert.ErrorContains(t, err, "attempted to retrieve read-write set for not-closed when done has not been called")
	err = vault1.DiscardTx(context.Background(), "not-closed", "")
	assert.EqualError(t, err, "attempted to retrieve read-write set for not-closed when done has not been called")

	// as a sanity-check we close it now and will be able to discard it
	ncrwset.Done()
	err = vault1.DiscardTx(context.Background(), "not-closed", "pineapple")
	assert.NoError(t, err)
	vc, message, err := vault1.Status(context.Background(), "not-closed")
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", message)
	assert.Equal(t, invalid, vc)

	_, err = vault1.NewRWSetFromBytes(context.Background(), "bogus", []byte("barf"))
	assert.Contains(t, err.Error(), "provided invalid read-write set bytes")

	code, _, err := vault1.Status(context.Background(), "unknown-txid")
	assert.NoError(t, err)
	assert.Equal(t, unknown, code)
}

func TTestMerge(t *testing.T, ddb driver2.VaultStore, vp artifactsProvider) {
	ns := "namespace"
	k1 := "key1"
	k2 := "key2"
	k3 := "key3"
	txid := "txid"
	ne1Key := "notexist1"
	ne2Key := "notexist2"

	// create DB and kvs
	vault2, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	err = ddb.Store(context.Background(), nil, driver.Writes{
		ns: map[driver.PKey]driver.VaultValue{
			k1: {Raw: []byte("v1"), Version: versionBlockTxNumToBytes(35, 1)},
		},
	}, nil)
	assert.NoError(t, err)

	rws, err := vault2.NewRWSet(context.Background(), txid)
	assert.NoError(t, err)
	defer rws.Done()
	v, err := rws.GetState(ns, k1)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)
	v, err = rws.GetState(ns, ne1Key)
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)
	err = rws.SetState(ns, k2, []byte("v2"))
	assert.NoError(t, err)
	err = rws.SetStateMetadata(ns, k3, map[string][]byte{"k3": []byte("v3")})
	assert.NoError(t, err)

	rwsb := &ReadWriteSet{
		ReadSet: ReadSet{
			Reads:        map[driver.Namespace]NamespaceReads{},
			OrderedReads: map[string][]string{},
		},
		WriteSet: WriteSet{
			Writes:        map[string]NamespaceWrites{},
			OrderedWrites: map[string][]string{},
		},
		MetaWriteSet: MetaWriteSet{
			MetaWrites: map[string]KeyedMetaWrites{},
		},
	}
	rwsb.ReadSet.Add(ns, k1, versionBlockTxNumToBytes(35, 1))
	rwsb.ReadSet.Add(ns, ne2Key, nil)
	assert.NoError(t, rwsb.WriteSet.Add(ns, k1, []byte("newv1")))
	assert.NoError(t, rwsb.MetaWriteSet.Add(ns, k1, map[string][]byte{"k1": []byte("v1")}))
	m := vp.NewMarshaller()
	rwsBytes, err := m.Marshal("pineapple", rwsb)
	assert.NoError(t, err)

	rw := rws.(*Interceptor[ValidationCode]).RWs()
	err = rws.AppendRWSet(rwsBytes)
	assert.NoError(t, err)
	assert.Equal(t, NamespaceKeyedMetaWrites{
		"namespace": {
			"key1": {"k1": []byte("v1")},
			"key3": {"k3": []byte("v3")},
		},
	}, rw.MetaWrites)
	assert.Equal(t, Writes{"namespace": {
		"key1": []byte("newv1"),
		"key2": []byte("v2"),
	}}, rw.Writes)
	assert.Equal(t, Reads{
		"namespace": {
			"key1":      versionBlockTxNumToBytes(35, 1),
			"notexist1": nil,
			"notexist2": nil,
		},
	}, rw.Reads)

	rwsb = &ReadWriteSet{
		ReadSet: ReadSet{
			Reads:        map[driver.Namespace]NamespaceReads{},
			OrderedReads: map[string][]string{},
		},
	}
	rwsb.ReadSet.Add(ns, k1, versionBlockTxNumToBytes(36, 1))
	rwsBytes, err = m.Marshal("pineapple", rwsb)
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at version [[0 0 0 36 0 0 0 1]], current value at version [[0 0 0 35 0 0 0 1]]")

	rwsb = &ReadWriteSet{
		WriteSet: WriteSet{
			Writes:        map[string]NamespaceWrites{},
			OrderedWrites: map[string][]string{},
		},
	}
	assert.NoError(t, rwsb.WriteSet.Add(ns, k2, []byte("v2")))
	rwsBytes, err = m.Marshal("pineapple", rwsb)
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.EqualError(t, err, "duplicate write entry for key namespace:key2")

	err = rws.AppendRWSet([]byte("barf"))
	assert.Contains(t, err.Error(), "provided invalid read-write set bytes")

	txRWSet := &rwset.TxReadWriteSet{
		NsRwset: []*rwset.NsReadWriteSet{
			{Rwset: []byte("barf")},
		},
	}
	rwsBytes, err = proto.Marshal(txRWSet)
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.Contains(t, err.Error(), "provided invalid read-write set bytes")

	rwsb = &ReadWriteSet{
		MetaWriteSet: MetaWriteSet{
			MetaWrites: map[string]KeyedMetaWrites{},
		},
	}
	assert.NoError(t, rwsb.MetaWriteSet.Add(ns, k3, map[string][]byte{"k": []byte("v")}))
	rwsBytes, err = m.Marshal("pineapple", rwsb)
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.EqualError(t, err, "duplicate metadata write entry for key namespace:key3")
}

func TTestInspector(t *testing.T, ddb driver2.VaultStore, vp artifactsProvider) {
	txid := "txid"
	ns := "ns"
	k1 := "\x00k1"
	k2 := "k2"

	// create DB and kvs
	aVault, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	err = ddb.Store(context.Background(), nil, driver.Writes{
		ns: map[driver.PKey]driver.VaultValue{
			k1: {Raw: []byte("v1"), Version: versionBlockTxNumToBytes(35, 1)},
		},
	}, nil)
	assert.NoError(t, err)

	rws, err := aVault.NewRWSet(context.Background(), txid)
	assert.NoError(t, err)
	v, err := rws.GetState(ns, k1)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)
	err = rws.SetState(ns, k2, []byte("v2"))
	assert.NoError(t, err)
	rws.Done()

	b, err := rws.Bytes()
	assert.NoError(t, err)

	i, err := aVault.InspectRWSet(context.Background(), b)
	assert.NoError(t, err)
	assert.NoError(t, i.IsValid())

	// the ephemeral rwset can "see" its own writes
	v, err = i.GetState(ns, k2)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2"), v)

	k, v, err := i.GetWriteAt(ns, 0)
	assert.NoError(t, err)
	assert.Equal(t, k2, k)
	assert.Equal(t, []byte("v2"), v)

	k, v, err = i.GetReadAt(ns, 0)
	assert.NoError(t, err)
	assert.Equal(t, k1, k)
	assert.Equal(t, []byte(nil), v)

	assert.Equal(t, 1, i.NumReads(ns))
	assert.Equal(t, 1, i.NumWrites(ns))
	assert.Equal(t, []string{"ns"}, i.Namespaces())

	i.Done()

	// check filtering
	i, err = aVault.InspectRWSet(context.Background(), b, "pineapple")
	assert.NoError(t, err)
	assert.NoError(t, i.IsValid())
	assert.Empty(t, i.Namespaces())
	i.Done()

	i, err = aVault.InspectRWSet(context.Background(), b, ns)
	assert.NoError(t, err)
	assert.NoError(t, i.IsValid())
	assert.Equal(t, []string{ns}, i.Namespaces())
	i.Done()
}

func TTestRun(t *testing.T, db1, db2 driver2.VaultStore, vp artifactsProvider) {
	ns := "namespace"
	k1 := "key1"
	k1Meta := "key1Meta"
	k2 := "key2"
	txid := "txid1"

	// create and populate 2 DBs
	err := db1.Store(context.Background(), nil,
		driver.Writes{
			ns: map[driver.PKey]driver.VaultValue{
				k1: {Raw: []byte("v1"), Version: versionBlockTxNumToBytes(35, 1)},
			},
		},
		driver.MetaWrites{
			ns: map[driver.PKey]driver.VaultMetadataValue{
				k1Meta: {Metadata: map[string][]byte{"metakey": []byte("metavalue")}},
			},
		})
	assert.NoError(t, err)

	err = db2.Store(context.Background(), nil,
		driver.Writes{
			ns: map[driver.PKey]driver.VaultValue{
				k1: {Raw: []byte("v1"), Version: versionBlockTxNumToBytes(35, 1)},
			},
		},
		driver.MetaWrites{
			ns: map[driver.PKey]driver.VaultMetadataValue{
				k1Meta: {Metadata: map[string][]byte{"metakey": []byte("metavalue")}},
			},
		})
	assert.NoError(t, err)

	compare(t, ns, db1, db2)

	// create 2 vaults
	vault1, err := vp.NewNonCachedVault(db1)
	assert.NoError(t, err)
	vault2, err := vp.NewNonCachedVault(db2)
	assert.NoError(t, err)

	rws, err := vault1.NewRWSet(context.Background(), txid)
	assert.NoError(t, err)

	rws2, err := vault2.NewRWSet(context.Background(), txid) // here 1
	assert.NoError(t, err)
	rws2.Done()

	// GET K1
	v, err := rws.GetState(ns, k1, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k1 /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	// GET K1Meta
	vMap, err := rws.GetStateMetadata(ns, k1Meta, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	// SET K1
	err = rws.SetState(ns, k1, []byte("v1_updated"))
	assert.NoError(t, err)

	// GET K1 after setting it
	v, err = rws.GetState(ns, k1, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1_updated"), v)

	v, err = rws.GetState(ns, k1, driver.FromStorage)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1_updated"), v)

	// SET K1
	err = rws.SetStateMetadata(ns, k1Meta, map[string][]byte{"newmetakey": []byte("newmetavalue")})
	assert.NoError(t, err)

	// GET K1Meta after setting it
	vMap, err = rws.GetStateMetadata(ns, k1Meta, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	// GET K2
	v, err = rws.GetState(ns, k2, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, driver.FromStorage)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, driver.FromBoth)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// SET K2
	err = rws.SetState(ns, k2, []byte("v2_updated"))
	assert.NoError(t, err)

	// GET K2 after setting it
	v, err = rws.GetState(ns, k2, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	v, err = rws.GetState(ns, k2, driver.FromStorage)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	// we're done with this read-write set, we serialise it
	rws.Done()
	rwsBytes, err := rws.Bytes()
	assert.NoError(t, err)
	assert.NotNil(t, rwsBytes)

	assert.NoError(t, vault1.Match(context.Background(), txid, rwsBytes))
	assert.Error(t, vault1.Match(context.Background(), txid, []byte("pineapple")))

	// we open the read-write set fabric.From the other kvs
	rws, err = vault2.NewRWSetFromBytes(context.Background(), txid, rwsBytes)
	assert.NoError(t, err)

	assert.Equal(t, []string{ns}, rws.Namespaces())
	// we check reads positionally
	nReads := rws.NumReads(ns)
	assert.Equal(t, 3, nReads)
	rKey, rKeyVal, err := rws.GetReadAt(ns, 0)
	assert.NoError(t, err)
	assert.Equal(t, k1, rKey)
	assert.Equal(t, []byte("v1"), rKeyVal)
	rKey, rKeyVal, err = rws.GetReadAt(ns, 1)
	assert.NoError(t, err)
	assert.Equal(t, k1Meta, rKey)
	assert.Empty(t, rKeyVal)
	rKey, rKeyVal, err = rws.GetReadAt(ns, 2)
	assert.NoError(t, err)
	assert.Equal(t, k2, rKey)
	assert.Equal(t, []byte(nil), rKeyVal)
	_, _, err = rws.GetReadAt(ns, 3)
	assert.EqualError(t, err, "no read at position 3 for namespace namespace")
	nReads = rws.NumReads("barf")
	assert.Equal(t, 0, nReads)
	// we check writes positionally
	nWrites := rws.NumWrites(ns)
	assert.Equal(t, 2, nWrites)
	nWrites = rws.NumWrites("barfobarfs")
	assert.Equal(t, 0, nWrites)
	wKey, wKeyVal, err := rws.GetWriteAt(ns, 0)
	assert.NoError(t, err)
	assert.Equal(t, k1, wKey)
	assert.Equal(t, []byte("v1_updated"), wKeyVal)
	wKey, wKeyVal, err = rws.GetWriteAt(ns, 1)
	assert.NoError(t, err)
	assert.Equal(t, k2, wKey)
	assert.Equal(t, []byte("v2_updated"), wKeyVal)
	_, _, err = rws.GetWriteAt(ns, 2)
	assert.EqualError(t, err, "no write at position 2 for namespace namespace")

	// GET K1
	v, err = rws.GetState(ns, k1, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1_updated"), v)

	v, err = rws.GetState(ns, k1, driver.FromStorage)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1_updated"), v)

	// GET K2
	v, err = rws.GetState(ns, k2, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	v, err = rws.GetState(ns, k2, driver.FromStorage)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	// GET K1Meta
	vMap, err = rws.GetStateMetadata(ns, k1Meta, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	// DELETE K1
	err = rws.DeleteState(ns, k1)
	assert.NoError(t, err)

	// GET K1
	v, err = rws.GetState(ns, k1, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k1, driver.FromStorage)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, driver.FromBoth)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// we're done with this read-write set, we serialise it
	rws.Done()
	rwsBytes, err = rws.Bytes()
	assert.NoError(t, err)
	assert.NotNil(t, rwsBytes)

	// we open the read-write set fabric.From the first kvs again
	rws, err = vault1.NewRWSetFromBytes(context.Background(), txid, rwsBytes)
	assert.NoError(t, err)

	assert.Equal(t, []string{ns}, rws.Namespaces())
	// we check reads positionally
	nReads = rws.NumReads(ns)
	assert.Equal(t, 3, nReads)
	rKey, rKeyVal, err = rws.GetReadAt(ns, 0)
	assert.NoError(t, err)
	assert.Equal(t, k1, rKey)
	assert.Equal(t, []byte("v1"), rKeyVal)
	rKey, rKeyVal, err = rws.GetReadAt(ns, 1)
	assert.NoError(t, err)
	assert.Equal(t, k1Meta, rKey)
	assert.Empty(t, rKeyVal)
	rKey, rKeyVal, err = rws.GetReadAt(ns, 2)
	assert.NoError(t, err)
	assert.Equal(t, k2, rKey)
	assert.Equal(t, []byte(nil), rKeyVal)
	_, _, err = rws.GetReadAt(ns, 3)
	assert.EqualError(t, err, "no read at position 3 for namespace namespace")
	nReads = rws.NumReads("barf")
	assert.Equal(t, 0, nReads)
	// we check writes positionally
	nWrites = rws.NumWrites(ns)
	assert.Equal(t, 2, nWrites)
	nWrites = rws.NumWrites("barfobarfs")
	assert.Equal(t, 0, nWrites)
	wKey, wKeyVal, err = rws.GetWriteAt(ns, 0)
	assert.NoError(t, err)
	assert.Equal(t, k1, wKey)
	assert.Equal(t, []byte(nil), wKeyVal)
	wKey, wKeyVal, err = rws.GetWriteAt(ns, 1)
	assert.NoError(t, err)
	assert.Equal(t, k2, wKey)
	assert.Equal(t, []byte("v2_updated"), wKeyVal)
	_, _, err = rws.GetWriteAt(ns, 2)
	assert.EqualError(t, err, "no write at position 2 for namespace namespace")

	// GET K2
	v, err = rws.GetState(ns, k2, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	v, err = rws.GetState(ns, k2, driver.FromStorage)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	// GET K1
	v, err = rws.GetState(ns, k1, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k1, driver.FromStorage)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, driver.FromBoth)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// GET K1Meta
	vMap, err = rws.GetStateMetadata(ns, k1Meta, driver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta, driver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	// we're done with this read-write set
	rws.Done()

	compare(t, ns, db1, db2)

	// we expect a busy txid in the Store
	code, _, err := vault1.Status(context.Background(), txid)
	assert.NoError(t, err)
	assert.Equal(t, busy, code)
	code, _, err = vault2.Status(context.Background(), txid)
	assert.NoError(t, err)
	assert.Equal(t, busy, code)

	compare(t, ns, db1, db2)

	// we commit it in both
	err = vault1.CommitTX(context.TODO(), txid, 35, 2)
	assert.NoError(t, err)
	err = vault2.CommitTX(context.TODO(), txid, 35, 2)
	assert.NoError(t, err)

	// all interceptors should be gone
	assert.Len(t, vault1.interceptors, 0)
	assert.Len(t, vault2.interceptors, 0)

	compare(t, ns, db1, db2)
	// we expect a valid txid in the Store
	code, _, err = vault1.Status(context.Background(), txid)
	assert.NoError(t, err)
	assert.Equal(t, valid, code)
	code, _, err = vault2.Status(context.Background(), txid)
	assert.NoError(t, err)
	assert.Equal(t, valid, code)

	compare(t, ns, db1, db2)

	vv1, err := db1.GetState(context.Background(), ns, k1)
	assert.NoError(t, err)
	assert.Nil(t, vv1.Raw)
	assert.Zero(t, vv1.Version)

	vv2, err := db2.GetState(context.Background(), ns, k1)
	assert.NoError(t, err)
	assert.Equal(t, vv1, vv2)

	vv1, err = db1.GetState(context.Background(), ns, k2)
	assert.NoError(t, err)
	vv2, err = db2.GetState(context.Background(), ns, k2)
	assert.NoError(t, err)
	assert.Equal(t, &driver.VaultRead{Key: "key2", Raw: []byte("v2_updated"), Version: versionBlockTxNumToBytes(35, 2)}, vv1)
	assert.Equal(t, vv1, vv2)

	meta1, ver1, err := db1.GetStateMetadata(context.Background(), ns, k1Meta)
	assert.NoError(t, err)
	versionMarshaller := BlockTxIndexVersionMarshaller{}
	b1, t1, err := versionMarshaller.FromBytes(ver1)
	assert.NoError(t, err)
	meta2, ver2, err := db2.GetStateMetadata(context.Background(), ns, k1Meta)
	assert.NoError(t, err)
	b2, t2, err := versionMarshaller.FromBytes(ver2)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, meta1)
	assert.Equal(t, uint64(35), b1)
	assert.Equal(t, uint64(2), t1)
	assert.Equal(t, meta1, meta2)
	assert.Equal(t, b1, b2)
	assert.Equal(t, t1, t2)
}

func compare(t *testing.T, ns string, db1, db2 driver2.VaultStore) {
	// we expect the underlying databases to be identical
	itr, err := db1.GetAllStates(context.Background(), ns)
	assert.NoError(t, err)
	res1, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	slices.SortFunc(res1, byKey)

	itr, err = db2.GetAllStates(context.Background(), ns)
	assert.NoError(t, err)
	res2, err := collections.ReadAll(itr)
	assert.NoError(t, err)
	slices.SortFunc(res2, byKey)

	assert.Equal(t, res1, res2)
}

func byKey(a, b driver.VaultRead) int { return strings.Compare(a.Key, b.Key) }

func versionBlockTxNumToBytes(Block driver.BlockNum, TxNum driver.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(Block))
	binary.BigEndian.PutUint32(buf[4:], uint32(TxNum))
	return buf
}
