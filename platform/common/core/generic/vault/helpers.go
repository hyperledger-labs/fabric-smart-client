/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/stretchr/testify/assert"
)

type ValidationCode = int

const (
	_ ValidationCode = iota
	valid
	invalid
	busy
	unknown
)

var RemoveNils func(items []VersionedRead) []VersionedRead

type VCProvider struct{}

func (p *VCProvider) ToInt32(code ValidationCode) int32 { return int32(code) }
func (p *VCProvider) FromInt32(code int32) ValidationCode {
	return ValidationCode(code)
}
func (p *VCProvider) Unknown() ValidationCode  { return unknown }
func (p *VCProvider) Busy() ValidationCode     { return busy }
func (p *VCProvider) Valid() ValidationCode    { return valid }
func (p *VCProvider) Invalid() ValidationCode  { return invalid }
func (p *VCProvider) NotFound() ValidationCode { return 0 }

type artifactsProvider interface {
	NewCachedVault(ddb VersionedPersistence) (*Vault[ValidationCode], error)
	NewNonCachedVault(ddb VersionedPersistence) (*Vault[ValidationCode], error)
	NewMarshaller() Marshaller
}

var SingleDBCases = []struct {
	Name string
	Fn   func(*testing.T, VersionedPersistence, artifactsProvider)
}{
	{"Merge", TTestMerge},
	{"Inspector", TTestInspector},
	{"InterceptorErr", TTestInterceptorErr},
	{"InterceptorConcurrency", TTestInterceptorConcurrency},
	{"QueryExecutor", TTestQueryExecutor},
	{"ShardLikeCommit", TTestShardLikeCommit},
	{"VaultErr", TTestVaultErr},
	{"ParallelVaults", TTestParallelVaults},
	{"Deadlock", TTestDeadlock},
}

var DoubleDBCases = []struct {
	Name string
	Fn   func(*testing.T, VersionedPersistence, VersionedPersistence, artifactsProvider)
}{
	{"Run", TTestRun},
}

func TTestInterceptorErr(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
	vault1, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	rws, err := vault1.NewRWSet("txid")
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

	rws, err = vault1.NewRWSet("validtxid")
	assert.NoError(t, err)
	rws.Done()
	err = vault1.CommitTX(context.TODO(), "validtxid", 2, 3)
	assert.NoError(t, err)
	rws, err = vault1.NewRWSet("validtxid")
	assert.NoError(t, err)
	err = rws.IsValid()
	assert.EqualError(t, err, "duplicate txid validtxid")
}

func TTestInterceptorConcurrency(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
	ns := "namespace"
	k := "key1"
	mk := "meyakey1"

	vault1, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	rws, err := vault1.NewRWSet("txid")
	assert.NoError(t, err)

	v, err := rws.GetState(ns, k)
	assert.NoError(t, err)
	assert.Nil(t, v)

	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, k, VersionedValue{Raw: []byte("val"), Version: toBytes(35, 1)})
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	_, _, err = rws.GetReadAt(ns, 0)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at fver [[]], current value at fver [[0 0 0 35 0 0 0 1]]")

	_, err = rws.GetState(ns, k)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at fver [[]], current value at fver [[0 0 0 35 0 0 0 1]]")

	mv, err := rws.GetStateMetadata(ns, mk)
	assert.NoError(t, err)
	assert.Nil(t, mv)

	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetStateMetadata(ns, mk, map[string][]byte{"k": []byte("v")}, nil)
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	_, err = rws.GetStateMetadata(ns, mk)
	assert.EqualError(t, err, "invalid metadata read: previous value returned at fver [[0 0 0 0 0 0 0 0]], current value at fver [[0 0 0 36 0 0 0 2]]")
}

func TTestParallelVaults(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
	ns := "namespace"
	k := "key1"
	mk := "meyakey1"
	txID := "txid"

	vault1, err := vp.NewCachedVault(ddb)
	assert.NoError(t, err)

	vault2, err := vp.NewCachedVault(&duplicateErrorPersistence{ddb})
	assert.NoError(t, err)

	rws1, err := vault1.NewRWSet(txID)
	assert.NoError(t, err)
	assert.NoError(t, rws1.SetState(ns, k, []byte("val_v1")))
	assert.NoError(t, rws1.SetStateMetadata(ns, mk, map[string][]byte{"k1": []byte("mval1_v1")}))
	rws1.Done()

	rws2, err := vault2.NewRWSet(txID)
	assert.NoError(t, err)
	assert.NoError(t, rws2.SetState(ns, k, []byte("val_v2")))
	assert.NoError(t, rws2.SetStateMetadata(ns, mk, map[string][]byte{"k1": []byte("mval1_v2"), "k2": []byte("mval2_v2")}))
	rws2.Done()

	val, mval, txNum, blkNum, err := queryVault(vault1, ns, k, mk)
	assert.NoError(t, err)
	assert.Nil(t, val)
	assert.Nil(t, mval)
	assert.Zero(t, txNum)
	assert.Zero(t, blkNum)

	val, mval, txNum, blkNum, err = queryVault(vault2, ns, k, mk)
	assert.NoError(t, err)
	assert.Nil(t, val)
	assert.Nil(t, mval)
	assert.Zero(t, txNum)
	assert.Zero(t, blkNum)

	assert.NoError(t, vault1.CommitTX(context.TODO(), txID, 1, 2))
	assert.NoError(t, vault2.CommitTX(context.TODO(), txID, 1, 2))

	val, mval, txNum, blkNum, err = queryVault(vault1, ns, k, mk)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val_v1"), val)
	assert.Equal(t, map[string][]byte{"k1": []byte("mval1_v1")}, mval)
	assert.Equal(t, uint64(2), txNum)
	assert.Equal(t, uint64(1), blkNum)

	val, mval, txNum, blkNum, err = queryVault(vault2, ns, k, mk)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val_v1"), val)
	assert.Equal(t, map[string][]byte{"k1": []byte("mval1_v1")}, mval)
	assert.Equal(t, uint64(2), txNum)
	assert.Equal(t, uint64(1), blkNum)
}

func TTestDeadlock(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
	ns := "namespace"
	k := "key1"
	mk := "meyakey1"
	txID := "txid"
	deadlockDB := &deadlockErrorPersistence{ddb, 3, k}

	vault1, err := vp.NewCachedVault(deadlockDB)
	assert.NoError(t, err)

	rws1, err := vault1.NewRWSet(txID)
	assert.NoError(t, err)
	assert.NoError(t, rws1.SetState(ns, k, []byte("val_v1")))
	assert.NoError(t, rws1.SetStateMetadata(ns, mk, map[string][]byte{"k1": []byte("mval1_v1")}))
	rws1.Done()

	val, mval, txNum, blkNum, err := queryVault(vault1, ns, k, mk)
	assert.NoError(t, err)
	assert.Nil(t, val)
	assert.Nil(t, mval)
	assert.Zero(t, txNum)
	assert.Zero(t, blkNum)

	assert.NoError(t, vault1.CommitTX(context.TODO(), txID, 1, 2))
	assert.Zero(t, deadlockDB.failures, "failed 3 times because of deadlock")

	val, mval, txNum, blkNum, err = queryVault(vault1, ns, k, mk)
	assert.NoError(t, err)
	assert.Equal(t, []byte("val_v1"), val)
	assert.Equal(t, map[string][]byte{"k1": []byte("mval1_v1")}, mval)
	assert.Equal(t, uint64(2), txNum)
	assert.Equal(t, uint64(1), blkNum)
}

func TTestQueryExecutor(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
	ns := "namespace"

	aVault, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)

	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, "k2", VersionedValue{Raw: []byte("k2_value"), Version: toBytes(35, 1)})
	assert.NoError(t, err)
	err = ddb.SetState(ns, "k3", VersionedValue{Raw: []byte("k3_value"), Version: toBytes(35, 2)})
	assert.NoError(t, err)
	err = ddb.SetState(ns, "k1", VersionedValue{Raw: []byte("k1_value"), Version: toBytes(35, 3)})
	assert.NoError(t, err)
	err = ddb.SetState(ns, "k111", VersionedValue{Raw: []byte("k111_value"), Version: toBytes(35, 4)})
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	qe, err := aVault.NewQueryExecutor()
	assert.NoError(t, err)
	defer qe.Done()

	v, err := qe.GetState(ns, "k1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("k1_value"), v)
	v, err = qe.GetState(ns, "barfobarfs")
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)

	itr, err := qe.GetStateRangeScanIterator(ns, "", "")
	assert.NoError(t, err)
	defer itr.Close()

	res := make([]VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 4)
	assert.ElementsMatch(t, []VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: toBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: toBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: toBytes(35, 1)},
		{Key: "k3", Raw: []byte("k3_value"), Version: toBytes(35, 2)},
	}, res)

	itr, err = ddb.GetStateRangeScanIterator(ns, "k1", "k3")
	assert.NoError(t, err)
	defer itr.Close()

	res = make([]VersionedRead, 0, 3)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 3)
	assert.Equal(t, []VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: toBytes(35, 3)},
		{Key: "k111", Raw: []byte("k111_value"), Version: toBytes(35, 4)},
		{Key: "k2", Raw: []byte("k2_value"), Version: toBytes(35, 1)},
	}, res)

	itr, err = ddb.GetStateSetIterator(ns, "k1", "k2", "k111")
	assert.NoError(t, err)
	defer itr.Close()

	res = make([]VersionedRead, 0, 3)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	assert.Len(t, res, 3)
	assert.ElementsMatch(t, []VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: toBytes(35, 3)},
		{Key: "k2", Raw: []byte("k2_value"), Version: toBytes(35, 1)},
		{Key: "k111", Raw: []byte("k111_value"), Version: toBytes(35, 4)},
	}, res)

	itr, err = ddb.GetStateSetIterator(ns, "k1", "k5")
	assert.NoError(t, err)
	defer itr.Close()

	res = make([]VersionedRead, 0, 2)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res = append(res, *n)
	}
	var expected = RemoveNils([]VersionedRead{
		{Key: "k1", Raw: []byte("k1_value"), Version: toBytes(35, 3)},
	})
	assert.Equal(t, expected, res)
}

func TTestShardLikeCommit(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
	ns := "namespace"
	k1 := "key1"
	k2 := "key2"

	// Populate the DB with some data at some height
	err := ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, k1, VersionedValue{Raw: []byte("k1val"), Version: toBytes(35, 1)})
	assert.NoError(t, err)
	err = ddb.SetState(ns, k2, VersionedValue{Raw: []byte("k2val"), Version: toBytes(37, 3)})
	assert.NoError(t, err)
	err = ddb.Commit()
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
	rwsb.ReadSet.Add(ns, k1, toBytes(35, 1))
	rwsb.ReadSet.Add(ns, k2, toBytes(37, 2))
	rwsb.WriteSet.Add(ns, k1, []byte("k1FromTxidInvalid"))
	rwsb.WriteSet.Add(ns, k2, []byte("k2FromTxidInvalid"))
	marshaller := vp.NewMarshaller()
	rwsBytes, err := marshaller.Marshal(rwsb)
	assert.NoError(t, err)

	// give it to the kvs and check whether it's valid - it won't be
	rwset, err := aVault.GetRWSet("txid-invalid", rwsBytes)
	assert.NoError(t, err)
	err = rwset.IsValid()
	assert.EqualError(t, err, "invalid read: vault at fver namespace:key2 [{[107 50 118 97 108] [0 0 0 37 0 0 0 3]}], read-write set at fver [[0 0 0 37 0 0 0 2]]")

	// close the read-write set, even in case of error
	rwset.Done()

	// check the status, it should be busy
	code, _, err := aVault.Status("txid-invalid")
	assert.NoError(t, err)
	assert.Equal(t, busy, code)

	// now in case of error we won't commit the read-write set, so we should discard it
	err = aVault.DiscardTx("txid-invalid", "")
	assert.NoError(t, err)

	// check the status, it should be invalid
	code, _, err = aVault.Status("txid-invalid")
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
	rwsb.ReadSet.Add(ns, k1, toBytes(35, 1))
	rwsb.ReadSet.Add(ns, k2, toBytes(37, 3))
	rwsb.WriteSet.Add(ns, k1, []byte("k1FromTxidValid"))
	rwsb.WriteSet.Add(ns, k2, []byte("k2FromTxidValid"))
	rwsBytes, err = marshaller.Marshal(rwsb)
	assert.NoError(t, err)

	// give it to the kvs and check whether it's valid - it will be
	rwset, err = aVault.GetRWSet("txid-valid", rwsBytes)
	assert.NoError(t, err)
	err = rwset.IsValid()
	assert.NoError(t, err)

	// close the read-write set
	rwset.Done()

	// presumably the cross-shard protocol continues...

	// check the status, it should be busy
	code, _, err = aVault.Status("txid-valid")
	assert.NoError(t, err)
	assert.Equal(t, busy, code)

	// we're now asked to really commit
	err = aVault.CommitTX(context.TODO(), "txid-valid", 38, 10)
	assert.NoError(t, err)

	// check the status, it should be valid
	code, _, err = aVault.Status("txid-valid")
	assert.NoError(t, err)
	assert.Equal(t, valid, code)

	// check the content of the kvs after that
	vv, err := ddb.GetState(ns, k1)
	assert.NoError(t, err)
	assert.Equal(t, VersionedValue{Raw: []byte("k1FromTxidValid"), Version: toBytes(38, 10)}, vv)

	vv, err = ddb.GetState(ns, k2)
	assert.NoError(t, err)
	assert.Equal(t, VersionedValue{Raw: []byte("k2FromTxidValid"), Version: toBytes(38, 10)}, vv)

	// all Interceptors should be gone
	assert.Len(t, aVault.Interceptors, 0)
}

func TTestVaultErr(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
	vault1, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	err = vault1.CommitTX(context.TODO(), "non-existent", 0, 0)
	assert.ErrorContains(t, err, "read-write set for txid non-existent could not be found")
	err = vault1.DiscardTx("non-existent", "")
	assert.EqualError(t, err, "read-write set for txid non-existent could not be found")

	rws := &ReadWriteSet{
		ReadSet: ReadSet{
			Reads:        map[driver.Namespace]NamespaceReads{},
			OrderedReads: map[string][]string{},
		},
	}
	rws.ReadSet.Add("pineapple", "key", toBytes(35, 1))
	m := vp.NewMarshaller()
	rwsBytes, err := m.Marshal(rws)
	assert.NoError(t, err)

	ncrwset, err := vault1.NewRWSet("not-closed")
	assert.NoError(t, err)
	_, err = vault1.NewRWSet("not-closed")
	assert.EqualError(t, err, "duplicate read-write set for txid not-closed")
	_, err = vault1.GetRWSet("not-closed", rwsBytes)
	assert.EqualError(t, err, "programming error: previous read-write set for not-closed has not been closed")
	err = vault1.CommitTX(context.TODO(), "not-closed", 0, 0)
	assert.ErrorContains(t, err, "attempted to retrieve read-write set for not-closed when done has not been called")
	err = vault1.DiscardTx("not-closed", "")
	assert.EqualError(t, err, "attempted to retrieve read-write set for not-closed when done has not been called")

	// as a sanity-check we close it now and will be able to discard it
	ncrwset.Done()
	err = vault1.DiscardTx("not-closed", "pineapple")
	assert.NoError(t, err)
	vc, message, err := vault1.Status("not-closed")
	assert.NoError(t, err)
	assert.Equal(t, "pineapple", message)
	assert.Equal(t, invalid, vc)

	_, err = vault1.GetRWSet("bogus", []byte("barf"))
	assert.Contains(t, err.Error(), "provided invalid read-write set bytes")

	code, _, err := vault1.Status("unknown-txid")
	assert.NoError(t, err)
	assert.Equal(t, unknown, code)
}

func TTestMerge(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
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
	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, k1, VersionedValue{Raw: []byte("v1"), Version: toBytes(35, 1)})
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	rws, err := vault2.NewInspector(txid)
	assert.NoError(t, err)
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
	rwsb.ReadSet.Add(ns, k1, toBytes(35, 1))
	rwsb.ReadSet.Add(ns, ne2Key, toBytes(0, 0))
	rwsb.WriteSet.Add(ns, k1, []byte("newv1"))
	rwsb.MetaWriteSet.Add(ns, k1, map[string][]byte{"k1": []byte("v1")})
	m := vp.NewMarshaller()
	rwsBytes, err := m.Marshal(rwsb)
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.NoError(t, err)
	assert.Equal(t, NamespaceKeyedMetaWrites{
		"namespace": {
			"key1": {"k1": []byte("v1")},
			"key3": {"k3": []byte("v3")},
		},
	}, rws.RWs().MetaWrites)
	assert.Equal(t, Writes{"namespace": {
		"key1": []byte("newv1"),
		"key2": []byte("v2"),
	}}, rws.RWs().Writes)
	assert.Equal(t, Reads{
		"namespace": {
			"key1":      toBytes(35, 1),
			"notexist1": nil,
			"notexist2": toBytes(0, 0),
		},
	}, rws.RWs().Reads)

	rwsb = &ReadWriteSet{
		ReadSet: ReadSet{
			Reads:        map[driver.Namespace]NamespaceReads{},
			OrderedReads: map[string][]string{},
		},
	}
	rwsb.ReadSet.Add(ns, k1, toBytes(36, 1))
	rwsBytes, err = m.Marshal(rwsb)
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at fver [[0 0 0 36 0 0 0 1]], current value at fver [[0 0 0 35 0 0 0 1]]")

	rwsb = &ReadWriteSet{
		WriteSet: WriteSet{
			Writes:        map[string]NamespaceWrites{},
			OrderedWrites: map[string][]string{},
		},
	}
	rwsb.WriteSet.Add(ns, k2, []byte("v2"))
	rwsBytes, err = m.Marshal(rwsb)
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
	rwsb.MetaWriteSet.Add(ns, k3, map[string][]byte{"k": []byte("v")})
	rwsBytes, err = m.Marshal(rwsb)
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.EqualError(t, err, "duplicate metadata write entry for key namespace:key3")
}

func TTestInspector(t *testing.T, ddb VersionedPersistence, vp artifactsProvider) {
	txid := "txid"
	ns := "ns"
	k1 := "k1"
	k2 := "k2"

	// create DB and kvs
	aVault, err := vp.NewNonCachedVault(ddb)
	assert.NoError(t, err)
	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, k1, VersionedValue{Raw: []byte("v1"), Version: toBytes(35, 1)})
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	rws, err := aVault.NewRWSet(txid)
	assert.NoError(t, err)
	v, err := rws.GetState(ns, k1)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)
	err = rws.SetState(ns, k2, []byte("v2"))
	assert.NoError(t, err)
	rws.Done()

	b, err := rws.Bytes()
	assert.NoError(t, err)

	i, err := aVault.InspectRWSet(b)
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
	i, err = aVault.InspectRWSet(b, "pineapple")
	assert.NoError(t, err)
	assert.NoError(t, i.IsValid())
	assert.Empty(t, i.Namespaces())
	i.Done()

	i, err = aVault.InspectRWSet(b, ns)
	assert.NoError(t, err)
	assert.NoError(t, i.IsValid())
	assert.Equal(t, []string{ns}, i.Namespaces())
	i.Done()
}

func TTestRun(t *testing.T, db1, db2 VersionedPersistence, vp artifactsProvider) {
	ns := "namespace"
	k1 := "key1"
	k1Meta := "key1Meta"
	k2 := "key2"
	txid := "txid1"

	// create and populate 2 DBs
	err := db1.BeginUpdate()
	assert.NoError(t, err)
	err = db1.SetState(ns, k1, VersionedValue{Raw: []byte("v1"), Version: toBytes(35, 1)})
	assert.NoError(t, err)
	err = db1.SetStateMetadata(ns, k1Meta, map[string][]byte{"metakey": []byte("metavalue")}, nil)
	assert.NoError(t, err)
	err = db1.Commit()
	assert.NoError(t, err)

	err = db2.BeginUpdate()
	assert.NoError(t, err)
	err = db2.SetState(ns, k1, VersionedValue{Raw: []byte("v1"), Version: toBytes(35, 1)})
	assert.NoError(t, err)
	err = db2.SetStateMetadata(ns, k1Meta, map[string][]byte{"metakey": []byte("metavalue")}, nil)
	assert.NoError(t, err)
	err = db2.Commit()
	assert.NoError(t, err)

	compare(t, ns, db1, db2)

	// create 2 vaults
	vault1, err := vp.NewNonCachedVault(db1)
	assert.NoError(t, err)
	vault2, err := vp.NewNonCachedVault(db2)
	assert.NoError(t, err)

	rws, err := vault1.NewRWSet(txid)
	assert.NoError(t, err)

	rws2, err := vault2.NewRWSet(txid)
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

	assert.NoError(t, vault1.Match(txid, rwsBytes))
	assert.Error(t, vault1.Match(txid, []byte("pineapple")))

	// we open the read-write set fabric.From the other kvs
	rws, err = vault2.GetRWSet(txid, rwsBytes)
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
	rws, err = vault1.GetRWSet(txid, rwsBytes)
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
	code, _, err := vault1.Status(txid)
	assert.NoError(t, err)
	assert.Equal(t, busy, code)
	code, _, err = vault2.Status(txid)
	assert.NoError(t, err)
	assert.Equal(t, busy, code)

	compare(t, ns, db1, db2)

	// we commit it in both
	err = vault1.CommitTX(context.TODO(), txid, 35, 2)
	assert.NoError(t, err)
	err = vault2.CommitTX(context.TODO(), txid, 35, 2)
	assert.NoError(t, err)

	// all Interceptors should be gone
	assert.Len(t, vault1.Interceptors, 0)
	assert.Len(t, vault2.Interceptors, 0)

	compare(t, ns, db1, db2)
	// we expect a valid txid in the Store
	code, _, err = vault1.Status(txid)
	assert.NoError(t, err)
	assert.Equal(t, valid, code)
	code, _, err = vault2.Status(txid)
	assert.NoError(t, err)
	assert.Equal(t, valid, code)

	compare(t, ns, db1, db2)

	vv1, err := db1.GetState(ns, k1)

	assert.NoError(t, err)
	vv2, err := db2.GetState(ns, k1)
	assert.NoError(t, err)
	assert.Nil(t, vv1.Raw)
	assert.Zero(t, vv1.Version)
	assert.Equal(t, vv1, vv2)

	vv1, err = db1.GetState(ns, k2)
	assert.NoError(t, err)
	vv2, err = db2.GetState(ns, k2)
	assert.NoError(t, err)
	assert.Equal(t, VersionedValue{Raw: []byte("v2_updated"), Version: toBytes(35, 2)}, vv1)
	assert.Equal(t, vv1, vv2)

	meta1, ver1, err := db1.GetStateMetadata(ns, k1Meta)
	assert.NoError(t, err)
	versionMarshaller := BlockTxIndexVersionMarshaller{}
	b1, t1, err := versionMarshaller.FromBytes(ver1)
	assert.NoError(t, err)
	meta2, ver2, err := db2.GetStateMetadata(ns, k1Meta)
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

func compare(t *testing.T, ns string, db1, db2 VersionedPersistence) {
	// we expect the underlying databases to be identical
	itr, err := db1.GetStateRangeScanIterator(ns, "", "")
	assert.NoError(t, err)
	defer itr.Close()

	res1 := make([]VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res1 = append(res1, *n)
	}
	itr, err = db2.GetStateRangeScanIterator(ns, "", "")
	assert.NoError(t, err)
	defer itr.Close()

	res2 := make([]VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res2 = append(res2, *n)
	}

	assert.Equal(t, res1, res2)
}

func queryVault(v *Vault[ValidationCode], ns driver.Namespace, key driver.PKey, mkey driver.MKey) (driver.RawValue, driver.Metadata, driver.TxNum, driver.BlockNum, error) {
	qe, err := v.NewQueryExecutor()
	defer qe.Done()
	if err != nil {
		return nil, nil, 0, 0, err
	}
	val, err := qe.GetState(ns, key)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	mval, kVersion, err := qe.GetStateMetadata(ns, mkey)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	blkNum, txNum, err := BlockTxIndexVersionMarshaller{}.FromBytes(kVersion)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	return val, mval, txNum, blkNum, nil
}

type deadlockErrorPersistence struct {
	VersionedPersistence
	failures int
	key      string
}

func (db *deadlockErrorPersistence) GetState(namespace driver.Namespace, key driver.PKey) (VersionedValue, error) {
	return db.VersionedPersistence.GetState(namespace, key)
}

func (db *deadlockErrorPersistence) GetStateRangeScanIterator(namespace driver.Namespace, startKey, endKey driver.PKey) (collections.Iterator[*VersionedRead], error) {
	return db.VersionedPersistence.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (db *deadlockErrorPersistence) GetStateSetIterator(ns driver.Namespace, keys ...driver.PKey) (collections.Iterator[*VersionedRead], error) {
	return db.VersionedPersistence.GetStateSetIterator(ns, keys...)
}

func (db *deadlockErrorPersistence) SetState(namespace driver.Namespace, key driver.PKey, value VersionedValue) error {
	if key == db.key && db.failures > 0 {
		db.failures--
		return DeadlockDetected
	}
	return db.VersionedPersistence.SetState(namespace, key, value)
}

func (db *deadlockErrorPersistence) SetStates(namespace driver.Namespace, kvs map[driver.PKey]VersionedValue) map[driver.PKey]error {
	errs := make(map[driver.PKey]error)
	for k, v := range kvs {
		if err := db.SetState(namespace, k, v); err != nil {
			errs[k] = err
		}
	}
	return errs
}

func (db *deadlockErrorPersistence) DeleteStates(namespace driver.Namespace, keys ...driver.PKey) map[driver.PKey]error {
	errs := make(map[driver.PKey]error)
	for _, key := range keys {
		if err := db.DeleteState(namespace, key); err != nil {
			errs[key] = err
		}
	}
	return errs
}

type duplicateErrorPersistence struct {
	VersionedPersistence
}

func (db *duplicateErrorPersistence) SetState(driver.Namespace, driver.PKey, VersionedValue) error {
	return UniqueKeyViolation
}

func (db *duplicateErrorPersistence) SetStates(_ driver.Namespace, kvs map[driver.PKey]VersionedValue) map[driver.PKey]error {
	errs := make(map[driver.PKey]error, len(kvs))
	for k := range kvs {
		errs[k] = UniqueKeyViolation
	}
	return errs
}

func (db *duplicateErrorPersistence) DeleteStates(namespace driver.Namespace, keys ...driver.PKey) map[driver.PKey]error {
	errs := make(map[driver.PKey]error)
	for _, key := range keys {
		if err := db.DeleteState(namespace, key); err != nil {
			errs[key] = err
		}
	}
	return errs
}

func (db *duplicateErrorPersistence) GetState(namespace driver.Namespace, key driver.PKey) (VersionedValue, error) {
	return db.VersionedPersistence.GetState(namespace, key)
}

func (db *duplicateErrorPersistence) GetStateRangeScanIterator(namespace driver.Namespace, startKey, endKey driver.PKey) (collections.Iterator[*VersionedRead], error) {
	return db.VersionedPersistence.GetStateRangeScanIterator(namespace, startKey, endKey)
}

func (db *duplicateErrorPersistence) GetStateSetIterator(ns driver.Namespace, keys ...driver.PKey) (collections.Iterator[*VersionedRead], error) {
	return db.VersionedPersistence.GetStateSetIterator(ns, keys...)
}

func toBytes(Block driver.BlockNum, TxNum driver.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(Block))
	binary.BigEndian.PutUint32(buf[4:], uint32(TxNum))
	return buf
}
