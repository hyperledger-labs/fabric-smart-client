/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/txidstore"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset/kvrwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/stretchr/testify/assert"
)

//go:generate counterfeiter -o mocks/config.go -fake-name Config . config

type config interface {
	db.Config
}

var tempDir string

func TestMerge(t *testing.T) {
	ns := "namespace"
	k1 := "key1"
	k2 := "key2"
	k3 := "key3"
	txid := "txid"
	ne1Key := "notexist1"
	ne2Key := "notexist2"

	// create DB and kvs
	ddb, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	tidstore, err := txidstore.NewTXIDStore(db.Unversioned(ddb))
	assert.NoError(t, err)
	vault := New(ddb, tidstore)
	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, k1, []byte("v1"), 35, 1)
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	rws, err := vault.NewRWSet(txid)
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

	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToReadSet(ns, k1, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: 35, TxNum: 1}))
	rwsb.AddToReadSet(ns, ne2Key, nil)
	rwsb.AddToWriteSet(ns, k1, []byte("newv1"))
	rwsb.AddToMetadataWriteSet(ns, k1, map[string][]byte{"k1": []byte("v1")})
	simRes, err := rwsb.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsBytes, err := simRes.GetPubSimulationBytes()
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.NoError(t, err)
	assert.Equal(t, namespaceKeyedMetaWrites{
		"namespace": {
			"key1": {"k1": []byte("v1")},
			"key3": {"k3": []byte("v3")},
		},
	}, rws.rws.metawrites)
	assert.Equal(t, writes{"namespace": {
		"key1": []byte("newv1"),
		"key2": []byte("v2"),
	}}, rws.rws.writes)
	assert.Equal(t, reads{
		"namespace": {
			"key1":      {block: 35, txnum: 1},
			"notexist1": {block: 0, txnum: 0},
			"notexist2": {block: 0, txnum: 0},
		},
	}, rws.rws.reads)

	rwsb = rwsetutil.NewRWSetBuilder()
	rwsb.AddToReadSet(ns, k1, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: 36, TxNum: 1}))
	simRes, err = rwsb.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsBytes, err = simRes.GetPubSimulationBytes()
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at version 35:1, current value at version 35:1")

	rwsb = rwsetutil.NewRWSetBuilder()
	rwsb.AddToWriteSet(ns, k2, []byte("v2"))
	simRes, err = rwsb.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsBytes, err = simRes.GetPubSimulationBytes()
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.EqualError(t, err, "duplicate write entry for key namespace:key2")

	err = rws.AppendRWSet([]byte("barf"))
	assert.Contains(t, err.Error(), "cannot parse invalid wire-format data")

	txRWSet := &rwset.TxReadWriteSet{
		NsRwset: []*rwset.NsReadWriteSet{
			{Rwset: []byte("barf")},
		},
	}
	rwsBytes, err = proto.Marshal(txRWSet)
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.Contains(t, err.Error(), "cannot parse invalid wire-format data")

	rwsb = rwsetutil.NewRWSetBuilder()
	rwsb.AddToMetadataWriteSet(ns, k3, map[string][]byte{"k": []byte("v")})
	simRes, err = rwsb.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsBytes, err = simRes.GetPubSimulationBytes()
	assert.NoError(t, err)

	err = rws.AppendRWSet(rwsBytes)
	assert.EqualError(t, err, "duplicate metadata write entry for key namespace:key3")
}

func TestInspector(t *testing.T) {
	txid := "txid"
	ns := "ns"
	k1 := "k1"
	k2 := "k2"

	// create DB and kvs
	ddb, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	tidstore, err := txidstore.NewTXIDStore(db.Unversioned(ddb))
	assert.NoError(t, err)
	vault := New(ddb, tidstore)
	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, k1, []byte("v1"), 35, 1)
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	rws, err := vault.NewRWSet(txid)
	assert.NoError(t, err)
	v, err := rws.GetState(ns, k1)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)
	err = rws.SetState(ns, k2, []byte("v2"))
	assert.NoError(t, err)
	rws.Done()

	b, err := rws.Bytes()
	assert.NoError(t, err)

	i, err := vault.InspectRWSet(b)
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
	i, err = vault.InspectRWSet(b, "pineapple")
	assert.NoError(t, err)
	assert.NoError(t, i.IsValid())
	assert.Empty(t, i.Namespaces())
	i.Done()

	i, err = vault.InspectRWSet(b, ns)
	assert.NoError(t, err)
	assert.NoError(t, i.IsValid())
	assert.Equal(t, []string{ns}, i.Namespaces())
	i.Done()
}

func testRun(t *testing.T, db1, db2 driver.VersionedPersistence) {
	ns := "namespace"
	k1 := "key1"
	k1Meta := "key1Meta"
	k2 := "key2"
	txid := "txid1"

	// create and populate 2 DBs
	err := db1.BeginUpdate()
	assert.NoError(t, err)
	err = db1.SetState(ns, k1, []byte("v1"), 35, 1)
	assert.NoError(t, err)
	err = db1.SetStateMetadata(ns, k1Meta, map[string][]byte{"metakey": []byte("metavalue")}, 35, 1)
	assert.NoError(t, err)
	err = db1.Commit()
	assert.NoError(t, err)

	err = db2.BeginUpdate()
	assert.NoError(t, err)
	err = db2.SetState(ns, k1, []byte("v1"), 35, 1)
	assert.NoError(t, err)
	err = db2.SetStateMetadata(ns, k1Meta, map[string][]byte{"metakey": []byte("metavalue")}, 35, 1)
	assert.NoError(t, err)
	err = db2.Commit()
	assert.NoError(t, err)

	compare(t, ns, db1, db2)

	// create 2 vaults
	tidstore1, err := txidstore.NewTXIDStore(db.Unversioned(db1))
	assert.NoError(t, err)
	tidstore2, err := txidstore.NewTXIDStore(db.Unversioned(db2))
	assert.NoError(t, err)
	vault1 := New(db1, tidstore1)
	vault2 := New(db2, tidstore2)

	rws, err := vault1.NewRWSet(txid)
	assert.NoError(t, err)

	rws2, err := vault2.NewRWSet(txid)
	assert.NoError(t, err)
	rws2.Done()

	// GET K1
	v, err := rws.GetState(ns, k1, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k1 /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	// GET K1Meta
	vMap, err := rws.GetStateMetadata(ns, k1Meta, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	// SET K1
	err = rws.SetState(ns, k1, []byte("v1_updated"))
	assert.NoError(t, err)

	// GET K1 after setting it
	v, err = rws.GetState(ns, k1, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1_updated"), v)

	v, err = rws.GetState(ns, k1, fdriver.FromStorage)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1_updated"), v)

	// SET K1
	err = rws.SetStateMetadata(ns, k1Meta, map[string][]byte{"newmetakey": []byte("newmetavalue")})
	assert.NoError(t, err)

	// GET K1Meta after setting it
	vMap, err = rws.GetStateMetadata(ns, k1Meta, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	// GET K2
	v, err = rws.GetState(ns, k2, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, fdriver.FromStorage)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// SET K2
	err = rws.SetState(ns, k2, []byte("v2_updated"))
	assert.NoError(t, err)

	// GET K2 after setting it
	v, err = rws.GetState(ns, k2, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	v, err = rws.GetState(ns, k2, fdriver.FromStorage)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, fdriver.FromBoth)
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
	assert.Equal(t, []byte(nil), rKeyVal)
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
	v, err = rws.GetState(ns, k1, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1_updated"), v)

	v, err = rws.GetState(ns, k1, fdriver.FromStorage)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1_updated"), v)

	// GET K2
	v, err = rws.GetState(ns, k2, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	v, err = rws.GetState(ns, k2, fdriver.FromStorage)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	// GET K1Meta
	vMap, err = rws.GetStateMetadata(ns, k1Meta, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	// DELETE K1
	err = rws.DeleteState(ns, k1)
	assert.NoError(t, err)

	// GET K1
	v, err = rws.GetState(ns, k1, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k1, fdriver.FromStorage)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, fdriver.FromBoth)
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
	assert.Equal(t, []byte(nil), rKeyVal)
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
	v, err = rws.GetState(ns, k2, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	v, err = rws.GetState(ns, k2, fdriver.FromStorage)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k2, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v)

	// GET K1
	v, err = rws.GetState(ns, k1, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Nil(t, v)

	v, err = rws.GetState(ns, k1, fdriver.FromStorage)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), v)

	v, err = rws.GetState(ns, k1, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Nil(t, v)

	// GET K1Meta
	vMap, err = rws.GetStateMetadata(ns, k1Meta, fdriver.FromIntermediate)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta /* , fabric.FromStorage */)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"metakey": []byte("metavalue")}, vMap)

	vMap, err = rws.GetStateMetadata(ns, k1Meta, fdriver.FromBoth)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, vMap)

	// we're done with this read-write set
	rws.Done()

	compare(t, ns, db1, db2)

	// we expect a busy txid in the store
	code, err := vault1.Status(txid)
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Busy, code)
	code, err = vault2.Status(txid)
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Busy, code)

	compare(t, ns, db1, db2)

	// we commit it in both
	err = vault1.CommitTX(txid, 35, 2)
	assert.NoError(t, err)
	err = vault2.CommitTX(txid, 35, 2)
	assert.NoError(t, err)

	// all interceptors should be gone
	assert.Len(t, vault1.interceptors, 0)
	assert.Len(t, vault2.interceptors, 0)

	compare(t, ns, db1, db2)
	// we expect a valid txid in the store
	code, err = vault1.Status(txid)
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Valid, code)
	code, err = vault2.Status(txid)
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Valid, code)

	compare(t, ns, db1, db2)

	v1, b1, t1, err := db1.GetState(ns, k1)
	assert.NoError(t, err)
	v2, b2, t2, err := db2.GetState(ns, k1)
	assert.NoError(t, err)
	assert.Nil(t, v1)
	assert.Equal(t, uint64(0), b1)
	assert.Equal(t, uint64(0), t1)
	assert.Equal(t, v1, v2)
	assert.Equal(t, b1, b2)
	assert.Equal(t, t1, t2)

	v1, b1, t1, err = db1.GetState(ns, k2)
	assert.NoError(t, err)
	v2, b2, t2, err = db2.GetState(ns, k2)
	assert.NoError(t, err)
	assert.Equal(t, []byte("v2_updated"), v1)
	assert.Equal(t, uint64(35), b1)
	assert.Equal(t, uint64(2), t1)
	assert.Equal(t, v1, v2)
	assert.Equal(t, b1, b2)
	assert.Equal(t, t1, t2)

	meta1, b1, t1, err := db1.GetStateMetadata(ns, k1Meta)
	assert.NoError(t, err)
	meta2, b2, t2, err := db2.GetStateMetadata(ns, k1Meta)
	assert.NoError(t, err)
	assert.Equal(t, map[string][]byte{"newmetakey": []byte("newmetavalue")}, meta1)
	assert.Equal(t, uint64(35), b1)
	assert.Equal(t, uint64(2), t1)
	assert.Equal(t, meta1, meta2)
	assert.Equal(t, b1, b2)
	assert.Equal(t, t1, t2)
}

func compare(t *testing.T, ns string, db1, db2 driver.VersionedPersistence) {
	// we expect the underlying databases to be identical
	itr, err := db1.GetStateRangeScanIterator(ns, "", "")
	defer itr.Close()
	assert.NoError(t, err)

	res1 := make([]driver.VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res1 = append(res1, *n)
	}
	itr, err = db2.GetStateRangeScanIterator(ns, "", "")
	defer itr.Close()
	assert.NoError(t, err)

	res2 := make([]driver.VersionedRead, 0, 4)
	for n, err := itr.Next(); n != nil; n, err = itr.Next() {
		assert.NoError(t, err)
		res2 = append(res2, *n)
	}

	assert.Equal(t, res1, res2)
}

func TestVaultInMem(t *testing.T) {
	db1, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	db2, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	testRun(t, db1, db2)
}

func TestVaultBadger(t *testing.T) {
	c := &mocks.Config{}
	c.UnmarshalKeyReturns(nil)
	c.IsSetReturns(false)
	db1, err := db.OpenVersioned(nil, "badger", filepath.Join(tempDir, "DB-TestVaultBadgerDB1"), c)
	assert.NoError(t, err)
	db2, err := db.OpenVersioned(nil, "badger", filepath.Join(tempDir, "DB-TestVaultBadgerDB2"), c)
	assert.NoError(t, err)
	defer db1.Close()
	defer db2.Close()

	testRun(t, db1, db2)
}

func TestVaultErr(t *testing.T) {
	ddb, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	tidstore, err := txidstore.NewTXIDStore(db.Unversioned(ddb))
	assert.NoError(t, err)
	vault1 := New(ddb, tidstore)
	err = vault1.CommitTX("non-existent", 0, 0)
	assert.EqualError(t, err, "read-write set for txid non-existent could not be found")
	err = vault1.DiscardTx("non-existent")
	assert.EqualError(t, err, "read-write set for txid non-existent could not be found")

	ncrwset, err := vault1.NewRWSet("not-closed")
	assert.NoError(t, err)
	_, err = vault1.NewRWSet("not-closed")
	assert.EqualError(t, err, "duplicate read-write set for txid not-closed")
	_, err = vault1.GetRWSet("not-closed", []byte(nil))
	assert.EqualError(t, err, "programming error: previous read-write set for not-closed has not been closed")
	err = vault1.CommitTX("not-closed", 0, 0)
	assert.EqualError(t, err, "attempted to retrieve read-write set for not-closed when done has not been called")
	err = vault1.DiscardTx("not-closed")
	assert.EqualError(t, err, "attempted to retrieve read-write set for not-closed when done has not been called")

	// as a sanity-check we close it now and will be able to discard it
	ncrwset.Done()
	err = vault1.DiscardTx("not-closed")
	assert.NoError(t, err)

	_, err = vault1.GetRWSet("bogus", []byte("barf"))
	assert.Contains(t, err.Error(), "cannot parse invalid wire-format data")

	txRWSet := &rwset.TxReadWriteSet{
		NsRwset: []*rwset.NsReadWriteSet{
			{Rwset: []byte("barf")},
		},
	}
	rwsb, err := proto.Marshal(txRWSet)
	assert.NoError(t, err)

	_, err = vault1.GetRWSet("bogus", rwsb)
	assert.Contains(t, err.Error(), "cannot parse invalid wire-format data")

	code, err := vault1.Status("unknown-txid")
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Unknown, code)
}

func TestInterceptorErr(t *testing.T) {
	ddb, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	tidstore, err := txidstore.NewTXIDStore(db.Unversioned(ddb))
	assert.NoError(t, err)
	vault1 := New(ddb, tidstore)
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
	err = vault1.CommitTX("validtxid", 2, 3)
	assert.NoError(t, err)
	rws, err = vault1.NewRWSet("validtxid")
	assert.NoError(t, err)
	err = rws.IsValid()
	assert.EqualError(t, err, "duplicate txid validtxid")
}

func TestInterceptorConcurrency(t *testing.T) {
	ns := "namespace"
	k := "key1"
	mk := "meyakey1"

	ddb, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	tidstore, err := txidstore.NewTXIDStore(db.Unversioned(ddb))
	assert.NoError(t, err)
	vault1 := New(ddb, tidstore)
	rws, err := vault1.NewRWSet("txid")
	assert.NoError(t, err)

	v, err := rws.GetState(ns, k)
	assert.NoError(t, err)
	assert.Nil(t, v)

	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, k, []byte("val"), 35, 1)
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	_, _, err = rws.GetReadAt(ns, 0)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at version 0:0, current value at version 35:1")

	_, err = rws.GetState(ns, k)
	assert.EqualError(t, err, "invalid read [namespace:key1]: previous value returned at version 0:0, current value at version 35:1")

	mv, err := rws.GetStateMetadata(ns, mk)
	assert.NoError(t, err)
	assert.Nil(t, mv)

	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetStateMetadata(ns, mk, map[string][]byte{"k": []byte("v")}, 36, 2)
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	_, err = rws.GetStateMetadata(ns, mk)
	assert.EqualError(t, err, "invalid metadata read: previous value returned at version 0:0, current value at version 36:2")
}

func TestQueryExecutor(t *testing.T) {
	ns := "namespace"

	ddb, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	tidstore, err := txidstore.NewTXIDStore(db.Unversioned(ddb))
	assert.NoError(t, err)
	vault := New(ddb, tidstore)

	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, "k2", []byte("k2_value"), 35, 1)
	assert.NoError(t, err)
	err = ddb.SetState(ns, "k3", []byte("k3_value"), 35, 2)
	assert.NoError(t, err)
	err = ddb.SetState(ns, "k1", []byte("k1_value"), 35, 3)
	assert.NoError(t, err)
	err = ddb.SetState(ns, "k111", []byte("k111_value"), 35, 4)
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	qe, err := vault.NewQueryExecutor()
	assert.NoError(t, err)
	defer qe.Done()

	v, err := qe.GetState(ns, "k1")
	assert.NoError(t, err)
	assert.Equal(t, []byte("k1_value"), v)
	v, err = qe.GetState(ns, "barfobarfs")
	assert.NoError(t, err)
	assert.Equal(t, []byte(nil), v)

	itr, err := qe.GetStateRangeScanIterator(ns, "", "")
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

	itr, err = ddb.GetStateRangeScanIterator(ns, "k1", "k3")
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

func TestShardLikeCommit(t *testing.T) {
	ns := "namespace"
	k1 := "key1"
	k2 := "key2"

	// Populate the DB with some data at some height
	ddb, err := db.OpenVersioned(nil, "memory", "", nil)
	assert.NoError(t, err)
	err = ddb.BeginUpdate()
	assert.NoError(t, err)
	err = ddb.SetState(ns, k1, []byte("k1val"), 35, 1)
	assert.NoError(t, err)
	err = ddb.SetState(ns, k2, []byte("k2val"), 37, 3)
	assert.NoError(t, err)
	err = ddb.Commit()
	assert.NoError(t, err)

	tidstore, err := txidstore.NewTXIDStore(db.Unversioned(ddb))
	assert.NoError(t, err)
	vault := New(ddb, tidstore)

	// SCENARIO 1: there is a read conflict in the proposed rwset
	// create the read-write set
	rwsb := rwsetutil.NewRWSetBuilder()
	rwsb.AddToReadSet(ns, k1, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: 35, TxNum: 1}))
	rwsb.AddToReadSet(ns, k2, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: 37, TxNum: 2}))
	rwsb.AddToWriteSet(ns, k1, []byte("k1FromTxidInvalid"))
	rwsb.AddToWriteSet(ns, k2, []byte("k2FromTxidInvalid"))
	simRes, err := rwsb.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsBytes, err := simRes.GetPubSimulationBytes()
	assert.NoError(t, err)

	// give it to the kvs and check whether it's valid - it won't be
	rwset, err := vault.GetRWSet("txid-invalid", rwsBytes)
	assert.NoError(t, err)
	err = rwset.IsValid()
	assert.EqualError(t, err, "invalid read: vault at version namespace:key2 37:3, read-write set at version 37:2")

	// close the read-write set, even in case of error
	rwset.Done()

	// check the status, it should be busy
	code, err := vault.Status("txid-invalid")
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Busy, code)

	// now in case of error we won't commit the read-write set, so we should discard it
	err = vault.DiscardTx("txid-invalid")
	assert.NoError(t, err)

	// check the status, it should be invalid
	code, err = vault.Status("txid-invalid")
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Invalid, code)

	// SCENARIO 2: there is no read conflict
	// create the read-write set
	rwsb = rwsetutil.NewRWSetBuilder()
	rwsb.AddToReadSet(ns, k1, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: 35, TxNum: 1}))
	rwsb.AddToReadSet(ns, k2, rwsetutil.NewVersion(&kvrwset.Version{BlockNum: 37, TxNum: 3}))
	rwsb.AddToWriteSet(ns, k1, []byte("k1FromTxidValid"))
	rwsb.AddToWriteSet(ns, k2, []byte("k2FromTxidValid"))

	simRes, err = rwsb.GetTxSimulationResults()
	assert.NoError(t, err)
	rwsBytes, err = simRes.GetPubSimulationBytes()
	assert.NoError(t, err)

	// give it to the kvs and check whether it's valid - it will be
	rwset, err = vault.GetRWSet("txid-valid", rwsBytes)
	assert.NoError(t, err)
	err = rwset.IsValid()
	assert.NoError(t, err)

	// close the read-write set
	rwset.Done()

	// presumably the cross-shard protocol continues...

	// check the status, it should be busy
	code, err = vault.Status("txid-valid")
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Busy, code)

	// we're now asked to really commit
	err = vault.CommitTX("txid-valid", 38, 10)
	assert.NoError(t, err)

	// check the status, it should be valid
	code, err = vault.Status("txid-valid")
	assert.NoError(t, err)
	assert.Equal(t, fdriver.Valid, code)

	// check the content of the kvs after that
	v, b, tx, err := ddb.GetState(ns, k1)
	assert.NoError(t, err)
	assert.Equal(t, []byte("k1FromTxidValid"), v)
	assert.Equal(t, uint64(38), b)
	assert.Equal(t, uint64(10), tx)

	v, b, tx, err = ddb.GetState(ns, k2)
	assert.NoError(t, err)
	assert.Equal(t, []byte("k2FromTxidValid"), v)
	assert.Equal(t, uint64(38), b)
	assert.Equal(t, uint64(10), tx)

	// all interceptors should be gone
	assert.Len(t, vault.interceptors, 0)
}

func TestMain(m *testing.M) {
	var err error
	tempDir, err = ioutil.TempDir("", "vault-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temporary directory: %v", err)
		os.Exit(-1)
	}
	defer os.RemoveAll(tempDir)

	m.Run()
}
