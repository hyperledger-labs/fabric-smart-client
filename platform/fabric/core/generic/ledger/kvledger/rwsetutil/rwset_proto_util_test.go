/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rwsetutil

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset"
	"github.com/hyperledger/fabric-protos-go-apiv2/ledger/rwset/kvrwset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger/version"
)

func TestTxRWSetMarshalUnmarshal(t *testing.T) {
	t.Parallel()
	txRwSet := &TxRwSet{}

	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "k0", EndKey: "k9", ItrExhausted: true}
	SetRawReads(rqi1, []*kvrwset.KVRead{
		{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}},
		{Key: "k2", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
	})

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "k00", EndKey: "k90", ItrExhausted: true}
	SetMerkleSummary(rqi2, &kvrwset.QueryReadsMerkleSummary{MaxDegree: 5, MaxLevel: 4, MaxLevelHashes: [][]byte{[]byte("Hash-1"), []byte("Hash-2")}})

	txRwSet.NsRwSets = []*NsRwSet{
		{NameSpace: "ns1", KvRwSet: &kvrwset.KVRWSet{
			Reads:            []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1},
			Writes:           []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
		}},

		{NameSpace: "ns2", KvRwSet: &kvrwset.KVRWSet{
			Reads:            []*kvrwset.KVRead{{Key: "key3", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi2},
			Writes:           []*kvrwset.KVWrite{{Key: "key3", IsDelete: false, Value: []byte("value3")}},
		}},

		{NameSpace: "ns3", KvRwSet: &kvrwset.KVRWSet{
			Reads:            []*kvrwset.KVRead{{Key: "key4", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			RangeQueriesInfo: nil,
			Writes:           []*kvrwset.KVWrite{{Key: "key4", IsDelete: false, Value: []byte("value4")}},
		}},
	}

	protoBytes, err := txRwSet.ToProtoBytes()
	require.NoError(t, err)
	txRwSet1 := &TxRwSet{}
	require.NoError(t, txRwSet1.FromProtoBytes(protoBytes))
	require.Len(t, txRwSet.NsRwSets, len(txRwSet1.NsRwSets))
	for i, rwset := range txRwSet.NsRwSets {
		require.Equal(t, txRwSet1.NsRwSets[i].NameSpace, rwset.NameSpace)
		require.True(t, proto.Equal(txRwSet1.NsRwSets[i].KvRwSet, rwset.KvRwSet), "proto messages are not equal")
		require.Equal(t, txRwSet1.NsRwSets[i].CollHashedRwSets, rwset.CollHashedRwSets)
	}
}

func TestTxRwSetConversion(t *testing.T) {
	t.Parallel()
	txRwSet := sampleTxRwSet()
	protoMsg, err := txRwSet.toProtoMsg()
	require.NoError(t, err)
	txRwSet1, err := TxRwSetFromProtoMsg(protoMsg)
	require.NoError(t, err)
	require.Len(t, txRwSet.NsRwSets, len(txRwSet1.NsRwSets))
	for i, rwset := range txRwSet.NsRwSets {
		require.Equal(t, txRwSet1.NsRwSets[i].NameSpace, rwset.NameSpace)
		require.True(t, proto.Equal(txRwSet1.NsRwSets[i].KvRwSet, rwset.KvRwSet), "proto messages are not equal")
		for j, hashedRwSet := range rwset.CollHashedRwSets {
			require.Equal(t, txRwSet1.NsRwSets[i].CollHashedRwSets[j].CollectionName, hashedRwSet.CollectionName)
			require.True(t, proto.Equal(txRwSet1.NsRwSets[i].CollHashedRwSets[j].HashedRwSet, hashedRwSet.HashedRwSet), "proto messages are not equal")
			require.Equal(t, txRwSet1.NsRwSets[i].CollHashedRwSets[j].PvtRwSetHash, hashedRwSet.PvtRwSetHash)
		}
	}
}

func TestNsRwSetConversion(t *testing.T) {
	t.Parallel()
	nsRwSet := sampleNsRwSet("ns-1")
	protoMsg, err := nsRwSet.toProtoMsg()
	require.NoError(t, err)
	nsRwSet1, err := nsRwSetFromProtoMsg(protoMsg)
	require.NoError(t, err)
	require.Equal(t, nsRwSet1.NameSpace, nsRwSet.NameSpace)
	require.True(t, proto.Equal(nsRwSet1.KvRwSet, nsRwSet.KvRwSet), "proto messages are not equal")
	for j, hashedRwSet := range nsRwSet.CollHashedRwSets {
		require.Equal(t, nsRwSet1.CollHashedRwSets[j].CollectionName, hashedRwSet.CollectionName)
		require.True(t, proto.Equal(nsRwSet1.CollHashedRwSets[j].HashedRwSet, hashedRwSet.HashedRwSet), "proto messages are not equal")
		require.Equal(t, nsRwSet1.CollHashedRwSets[j].PvtRwSetHash, hashedRwSet.PvtRwSetHash)
	}
}

func TestNsRWSetConversionNoCollHashedRWs(t *testing.T) {
	t.Parallel()
	nsRwSet := sampleNsRwSetWithNoCollHashedRWs("ns-1")
	protoMsg, err := nsRwSet.toProtoMsg()
	require.NoError(t, err)
	require.Nil(t, protoMsg.CollectionHashedRwset)
}

func TestCollHashedRwSetConversion(t *testing.T) {
	t.Parallel()
	collHashedRwSet := sampleCollHashedRwSet("coll-1")
	protoMsg, err := collHashedRwSet.toProtoMsg()
	require.NoError(t, err)
	collHashedRwSet1, err := collHashedRwSetFromProtoMsg(protoMsg)
	require.NoError(t, err)
	require.Equal(t, collHashedRwSet.CollectionName, collHashedRwSet1.CollectionName)
	require.True(t, proto.Equal(collHashedRwSet.HashedRwSet, collHashedRwSet1.HashedRwSet), "proto messages are not equal")
	require.Equal(t, collHashedRwSet.PvtRwSetHash, collHashedRwSet1.PvtRwSetHash)
}

func TestNumCollections(t *testing.T) {
	t.Parallel()
	var txRwSet *TxRwSet
	require.Equal(t, 0, txRwSet.NumCollections())         // nil TxRwSet
	require.Equal(t, 0, (&TxRwSet{}).NumCollections())    // empty TxRwSet
	require.Equal(t, 4, sampleTxRwSet().NumCollections()) // sample TxRwSet
}

func sampleTxRwSet() *TxRwSet {
	txRwSet := &TxRwSet{}
	txRwSet.NsRwSets = append(txRwSet.NsRwSets, sampleNsRwSet("ns-1"))
	txRwSet.NsRwSets = append(txRwSet.NsRwSets, sampleNsRwSet("ns-2"))
	return txRwSet
}

func sampleNsRwSet(ns string) *NsRwSet {
	nsRwSet := &NsRwSet{
		NameSpace: ns,
		KvRwSet:   sampleKvRwSet(),
	}
	nsRwSet.CollHashedRwSets = append(nsRwSet.CollHashedRwSets, sampleCollHashedRwSet("coll-1"))
	nsRwSet.CollHashedRwSets = append(nsRwSet.CollHashedRwSets, sampleCollHashedRwSet("coll-2"))
	return nsRwSet
}

func sampleNsRwSetWithNoCollHashedRWs(ns string) *NsRwSet {
	return &NsRwSet{NameSpace: ns, KvRwSet: sampleKvRwSet()}
}

func sampleKvRwSet() *kvrwset.KVRWSet {
	rqi1 := &kvrwset.RangeQueryInfo{StartKey: "k0", EndKey: "k9", ItrExhausted: true}
	SetRawReads(rqi1, []*kvrwset.KVRead{
		{Key: "k1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}},
		{Key: "k2", Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
	})

	rqi2 := &kvrwset.RangeQueryInfo{StartKey: "k00", EndKey: "k90", ItrExhausted: true}
	SetMerkleSummary(rqi2, &kvrwset.QueryReadsMerkleSummary{MaxDegree: 5, MaxLevel: 4, MaxLevelHashes: [][]byte{[]byte("Hash-1"), []byte("Hash-2")}})
	return &kvrwset.KVRWSet{
		Reads:            []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
		RangeQueriesInfo: []*kvrwset.RangeQueryInfo{rqi1},
		Writes:           []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
	}
}

func sampleCollHashedRwSet(collectionName string) *CollHashedRwSet {
	collHashedRwSet := &CollHashedRwSet{
		CollectionName: collectionName,
		HashedRwSet: &kvrwset.HashedRWSet{
			HashedReads: []*kvrwset.KVReadHash{
				{KeyHash: []byte("Key-1-hash"), Version: &kvrwset.Version{BlockNum: 1, TxNum: 2}},
				{KeyHash: []byte("Key-2-hash"), Version: &kvrwset.Version{BlockNum: 2, TxNum: 3}},
			},
			HashedWrites: []*kvrwset.KVWriteHash{
				{KeyHash: []byte("Key-3-hash"), ValueHash: []byte("value-3-hash"), IsDelete: false},
				{KeyHash: []byte("Key-4-hash"), ValueHash: []byte("value-4-hash"), IsDelete: true},
			},
		},
		PvtRwSetHash: []byte(collectionName + "-pvt-rwset-hash"),
	}
	return collHashedRwSet
}

// /////////////////////////////////////////////////////////////////////////////
// tests for private read-write set
// /////////////////////////////////////////////////////////////////////////////

func TestTxPvtRwSetConversion(t *testing.T) {
	t.Parallel()
	txPvtRwSet := sampleTxPvtRwSet()
	protoMsg, err := txPvtRwSet.ToProtoMsg()
	require.NoError(t, err)
	txPvtRwSet1, err := TxPvtRwSetFromProtoMsg(protoMsg)
	require.NoError(t, err)
	require.Len(t, txPvtRwSet.NsPvtRwSet, len(txPvtRwSet1.NsPvtRwSet))
	for i, rwset := range txPvtRwSet.NsPvtRwSet {
		require.Equal(t, txPvtRwSet1.NsPvtRwSet[i].NameSpace, rwset.NameSpace)
		for j, hashedRwSet := range rwset.CollPvtRwSets {
			require.Equal(t, txPvtRwSet1.NsPvtRwSet[i].CollPvtRwSets[j].CollectionName, hashedRwSet.CollectionName)
			require.True(t, proto.Equal(txPvtRwSet1.NsPvtRwSet[i].CollPvtRwSets[j].KvRwSet, hashedRwSet.KvRwSet), "proto messages are not equal")
		}
	}
}

func sampleTxPvtRwSet() *TxPvtRwSet {
	txPvtRwSet := &TxPvtRwSet{}
	txPvtRwSet.NsPvtRwSet = append(txPvtRwSet.NsPvtRwSet, sampleNsPvtRwSet("ns-1"))
	txPvtRwSet.NsPvtRwSet = append(txPvtRwSet.NsPvtRwSet, sampleNsPvtRwSet("ns-2"))
	return txPvtRwSet
}

func sampleNsPvtRwSet(ns string) *NsPvtRwSet {
	nsRwSet := &NsPvtRwSet{NameSpace: ns}
	nsRwSet.CollPvtRwSets = append(nsRwSet.CollPvtRwSets, sampleCollPvtRwSet("coll-1"))
	nsRwSet.CollPvtRwSets = append(nsRwSet.CollPvtRwSets, sampleCollPvtRwSet("coll-2"))
	return nsRwSet
}

func sampleCollPvtRwSet(collectionName string) *CollPvtRwSet {
	return &CollPvtRwSet{
		CollectionName: collectionName,
		KvRwSet: &kvrwset.KVRWSet{
			Reads:  []*kvrwset.KVRead{{Key: "key1", Version: &kvrwset.Version{BlockNum: 1, TxNum: 1}}},
			Writes: []*kvrwset.KVWrite{{Key: "key2", IsDelete: false, Value: []byte("value2")}},
		},
	}
}

func TestVersionConversion(t *testing.T) {
	t.Parallel()
	protoVer := &kvrwset.Version{BlockNum: 5, TxNum: 2}
	internalVer := version.NewHeight(5, 2)
	// convert proto to internal
	require.Nil(t, NewVersion(nil))
	require.Equal(t, internalVer, NewVersion(protoVer))

	// convert internal to proto
	require.Nil(t, newProtoVersion(nil))
	require.Equal(t, protoVer, newProtoVersion(internalVer))
}

func TestIsDelete(t *testing.T) {
	t.Parallel()
	t.Run("kvWrite", func(t *testing.T) {
		t.Parallel()
		kvWritesToBeInterpretedAsDelete := []*kvrwset.KVWrite{
			{Value: nil, IsDelete: true},
			{Value: nil, IsDelete: false},
			{Value: []byte{}, IsDelete: true},
			{Value: []byte{}, IsDelete: false},
		}

		for _, k := range kvWritesToBeInterpretedAsDelete {
			require.True(t, IsKVWriteDelete(k))
		}
	})

	t.Run("kvhashwrite", func(t *testing.T) {
		t.Parallel()
		kvHashesWritesToBeInterpretedAsDelete := []*kvrwset.KVWriteHash{
			{ValueHash: nil, IsDelete: true},
			{ValueHash: nil, IsDelete: false},
			{ValueHash: []byte{}, IsDelete: true},
			{ValueHash: []byte{}, IsDelete: false},
			{ValueHash: computeHash([]byte{}), IsDelete: true},
			{ValueHash: computeHash([]byte{}), IsDelete: false},
		}

		for _, k := range kvHashesWritesToBeInterpretedAsDelete {
			require.True(t, IsKVWriteHashDelete(k))
		}
	})
}

func TestGetPvtDataHash(t *testing.T) {
	t.Parallel()
	txRwSet := sampleTxRwSet()
	txRwSet.NsRwSets[1].CollHashedRwSets[1].PvtRwSetHash = []byte("ns-2-coll-2-hash")

	tests := []struct {
		name     string
		txRwSet  *TxRwSet
		ns, coll string
		want     []byte
	}{
		{name: "first namespace", txRwSet: txRwSet, ns: "ns-1", coll: "coll-2", want: []byte("coll-2-pvt-rwset-hash")},
		{name: "second namespace", txRwSet: txRwSet, ns: "ns-2", coll: "coll-2", want: []byte("ns-2-coll-2-hash")},
		{name: "unknown namespace", txRwSet: txRwSet, ns: "ns-3", coll: "coll-1"},
		{name: "unknown collection", txRwSet: txRwSet, ns: "ns-1", coll: "coll-3"},
		{name: "empty rwset", txRwSet: &TxRwSet{}, ns: "ns-1", coll: "coll-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.txRwSet.GetPvtDataHash(tc.ns, tc.coll))
		})
	}
}

func TestTxPvtRwSetMarshalUnmarshal(t *testing.T) {
	t.Parallel()
	txPvtRwSet := sampleTxPvtRwSet()
	protoBytes, err := txPvtRwSet.ToProtoBytes()
	require.NoError(t, err)
	txPvtRwSet1 := &TxPvtRwSet{}
	require.NoError(t, txPvtRwSet1.FromProtoBytes(protoBytes))
	require.Len(t, txPvtRwSet1.NsPvtRwSet, len(txPvtRwSet.NsPvtRwSet))
	for i, nsPvtRwSet := range txPvtRwSet.NsPvtRwSet {
		assert.Equal(t, nsPvtRwSet.NameSpace, txPvtRwSet1.NsPvtRwSet[i].NameSpace)
		require.Len(t, txPvtRwSet1.NsPvtRwSet[i].CollPvtRwSets, len(nsPvtRwSet.CollPvtRwSets))
		for j, collPvtRwSet := range nsPvtRwSet.CollPvtRwSets {
			assert.Equal(t, collPvtRwSet.CollectionName, txPvtRwSet1.NsPvtRwSet[i].CollPvtRwSets[j].CollectionName)
			assert.True(t, proto.Equal(collPvtRwSet.KvRwSet, txPvtRwSet1.NsPvtRwSet[i].CollPvtRwSets[j].KvRwSet), "proto messages are not equal")
		}
	}
}

func TestFromProtoBytesEmpty(t *testing.T) {
	t.Parallel()
	txRwSet := &TxRwSet{NsRwSets: []*NsRwSet{{NameSpace: "sentinel"}}}
	require.NoError(t, txRwSet.FromProtoBytes(nil))
	assert.Empty(t, txRwSet.NsRwSets)

	txPvtRwSet := &TxPvtRwSet{NsPvtRwSet: []*NsPvtRwSet{{NameSpace: "sentinel"}}}
	require.NoError(t, txPvtRwSet.FromProtoBytes(nil))
	assert.Empty(t, txPvtRwSet.NsPvtRwSet)
}

func TestTxRwSetFromProtoBytesError(t *testing.T) {
	t.Parallel()
	validRwSet := serializeTestProtoMsg(t, sampleKvRwSet())
	tests := []struct {
		name       string
		protoBytes []byte
	}{
		{name: "invalid bytes", protoBytes: []byte{0xff}},
		{name: "invalid namespace rwset", protoBytes: serializeTestProtoMsg(t, &rwset.TxReadWriteSet{
			NsRwset: []*rwset.NsReadWriteSet{{Namespace: "ns", Rwset: []byte{0xff}}},
		})},
		{name: "invalid collection hashed rwset", protoBytes: serializeTestProtoMsg(t, &rwset.TxReadWriteSet{
			NsRwset: []*rwset.NsReadWriteSet{{
				Namespace:             "ns",
				Rwset:                 validRwSet,
				CollectionHashedRwset: []*rwset.CollectionHashedReadWriteSet{{CollectionName: "coll", HashedRwset: []byte{0xff}}},
			}},
		})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sentinel := []*NsRwSet{{NameSpace: "sentinel"}}
			txRwSet := &TxRwSet{NsRwSets: sentinel}
			require.ErrorContains(t, txRwSet.FromProtoBytes(tc.protoBytes), "proto:")
			assert.Equal(t, sentinel, txRwSet.NsRwSets)
		})
	}
}

func TestTxPvtRwSetFromProtoBytesError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		protoBytes []byte
	}{
		{name: "invalid bytes", protoBytes: []byte{0xff}},
		{name: "invalid collection pvt rwset", protoBytes: serializeTestProtoMsg(t, &rwset.TxPvtReadWriteSet{
			NsPvtRwset: []*rwset.NsPvtReadWriteSet{{
				Namespace:          "ns",
				CollectionPvtRwset: []*rwset.CollectionPvtReadWriteSet{{CollectionName: "coll", Rwset: []byte{0xff}}},
			}},
		})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sentinel := []*NsPvtRwSet{{NameSpace: "sentinel"}}
			txPvtRwSet := &TxPvtRwSet{NsPvtRwSet: sentinel}
			require.ErrorContains(t, txPvtRwSet.FromProtoBytes(tc.protoBytes), "proto:")
			assert.Equal(t, sentinel, txPvtRwSet.NsPvtRwSet)
		})
	}
}

func TestToProtoBytesError(t *testing.T) {
	t.Parallel()
	invalidKvRwSet := func() *kvrwset.KVRWSet {
		return &kvrwset.KVRWSet{Writes: []*kvrwset.KVWrite{{Key: "\xff", Value: []byte("v")}}}
	}
	tests := []struct {
		name         string
		toProtoBytes func() ([]byte, error)
	}{
		{name: "invalid public rwset", toProtoBytes: (&TxRwSet{
			NsRwSets: []*NsRwSet{{NameSpace: "ns", KvRwSet: invalidKvRwSet()}},
		}).ToProtoBytes},
		{name: "invalid collection hashed rwset", toProtoBytes: (&TxRwSet{
			NsRwSets: []*NsRwSet{{
				NameSpace: "ns",
				KvRwSet:   sampleKvRwSet(),
				CollHashedRwSets: []*CollHashedRwSet{{
					CollectionName: "coll",
					HashedRwSet: &kvrwset.HashedRWSet{MetadataWrites: []*kvrwset.KVMetadataWriteHash{{
						KeyHash: []byte("key-hash"),
						Entries: []*kvrwset.KVMetadataEntry{{Name: "\xff"}},
					}}},
				}},
			}},
		}).ToProtoBytes},
		{name: "invalid private rwset", toProtoBytes: (&TxPvtRwSet{
			NsPvtRwSet: []*NsPvtRwSet{{
				NameSpace:     "ns",
				CollPvtRwSets: []*CollPvtRwSet{{CollectionName: "coll", KvRwSet: invalidKvRwSet()}},
			}},
		}).ToProtoBytes},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			protoBytes, err := tc.toProtoBytes()
			require.ErrorContains(t, err, "invalid UTF-8")
			assert.Nil(t, protoBytes)
		})
	}
}
