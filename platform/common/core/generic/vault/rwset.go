/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"sort"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/pkg/errors"
)

type ReadWriteSet struct {
	ReadSet
	WriteSet
	MetaWriteSet
}

func (rws *ReadWriteSet) populate(rwsetBytes []byte, txid core.TxID, namespaces ...core.Namespace) error {
	txRWSet := &rwset.TxReadWriteSet{}
	err := proto.Unmarshal(rwsetBytes, txRWSet)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes for txid %s, unmarshal failed", txid)
	}

	rwsIn, err := rwsetutil.TxRwSetFromProtoMsg(txRWSet)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes for txid %s, TxRwSetFromProtoMsg failed", txid)
	}

	namespaceSet := utils.NewSet(namespaces...)
	for _, nsrws := range rwsIn.NsRwSets {
		ns := nsrws.NameSpace

		// skip if not in the list of namespaces
		if !namespaceSet.Empty() && !namespaceSet.Contains(ns) {
			continue
		}

		for _, read := range nsrws.KvRwSet.Reads {
			bn := core.BlockNum(0)
			txn := core.TxNum(0)
			if read.Version != nil {
				bn = read.Version.BlockNum
				txn = read.Version.TxNum
			}
			rws.ReadSet.add(ns, read.Key, bn, txn)
		}

		for _, write := range nsrws.KvRwSet.Writes {
			if err := rws.WriteSet.add(ns, write.Key, write.Value); err != nil {
				return err
			}
		}

		for _, metaWrite := range nsrws.KvRwSet.MetadataWrites {
			metadata := map[string][]byte{}
			for _, entry := range metaWrite.Entries {
				metadata[entry.Name] = append([]byte(nil), entry.Value...)
			}

			if err := rws.MetaWriteSet.add(ns, metaWrite.Key, metadata); err != nil {
				return err
			}
		}
	}

	return nil
}

type MetaWrites map[string][]byte

func (r MetaWrites) Equals(o MetaWrites) error {
	return entriesEqual(r, o, bytes.Equal)
}

type KeyedMetaWrites map[string]MetaWrites

func (r KeyedMetaWrites) Equals(o KeyedMetaWrites) error {
	return entriesEqual(r, o, func(a, b MetaWrites) bool { return a.Equals(b) == nil })
}

type NamespaceKeyedMetaWrites map[string]KeyedMetaWrites

func (r NamespaceKeyedMetaWrites) equals(o NamespaceKeyedMetaWrites, nss ...string) error {
	return entriesEqual(r, o, func(a, b KeyedMetaWrites) bool { return a.Equals(b) == nil }, nss...)
}

type MetaWriteSet struct {
	MetaWrites NamespaceKeyedMetaWrites
}

func (w *MetaWriteSet) add(ns, key string, meta map[string][]byte) error {
	if err := keys.ValidateNs(ns); err != nil {
		return err
	}

	nsMap, in := w.MetaWrites[ns]
	if !in {
		nsMap = KeyedMetaWrites{}
		w.MetaWrites[ns] = nsMap
	}

	metadata := map[string][]byte{}
	for k, v := range meta {
		metadata[k] = append([]byte(nil), v...)
	}

	nsMap[key] = metadata

	return nil
}

func (w *MetaWriteSet) get(ns, key string) map[string][]byte {
	return w.MetaWrites[ns][key]
}

func (w *MetaWriteSet) in(ns, key string) bool {
	_, in := w.MetaWrites[ns][key]
	return in
}

func (w *MetaWriteSet) clear(ns string) {
	w.MetaWrites[ns] = KeyedMetaWrites{}
}

type NamespaceWrites map[string][]byte

func (r NamespaceWrites) Keys() []string {
	return utils.Keys(r)
}

func (r NamespaceWrites) Equals(o NamespaceWrites) error {
	return entriesEqual(r, o, bytes.Equal)
}

type Writes map[string]NamespaceWrites

func (r Writes) equals(o Writes, nss ...string) error {
	return entriesEqual(r, o, func(a, b NamespaceWrites) bool { return a.Equals(b) == nil }, nss...)
}

type WriteSet struct {
	Writes        Writes
	OrderedWrites map[string][]string
}

func (w *WriteSet) add(ns, key string, value []byte) error {
	if err := keys.ValidateNs(ns); err != nil {
		return err
	}

	nsMap, in := w.Writes[ns]
	if !in {
		nsMap = map[string][]byte{}

		w.Writes[ns] = nsMap
		w.OrderedWrites[ns] = make([]string, 0, 8)
	}

	nsMap[key] = append([]byte(nil), value...)
	w.OrderedWrites[ns] = append(w.OrderedWrites[ns], key)

	return nil
}

func (w *WriteSet) get(ns, key string) []byte {
	return w.Writes[ns][key]
}

func (w *WriteSet) getAt(ns string, i int) (key string, in bool) {
	slice := w.OrderedWrites[ns]
	if i < 0 || i > len(slice)-1 {
		return "", false
	}

	return slice[i], true
}

func (w *WriteSet) in(ns, key string) bool {
	_, in := w.Writes[ns][key]
	return in
}

func (w *WriteSet) clear(ns string) {
	w.Writes[ns] = map[string][]byte{}
	w.OrderedWrites[ns] = []string{}
}

type txPosition struct {
	Block core.BlockNum
	TxNum core.TxNum
}

type NamespaceReads map[string]txPosition

func (r NamespaceReads) Equals(o NamespaceReads) error {
	return entriesEqual(r, o, func(v, v2 txPosition) bool {
		return v.Block == v2.Block && v.TxNum == v2.TxNum
	})
}

type Reads map[core.Namespace]NamespaceReads

func (r Reads) equals(o Reads, nss ...core.Namespace) error {
	return entriesEqual(r, o, func(a, b NamespaceReads) bool { return a.Equals(b) == nil }, nss...)
}

type ReadSet struct {
	Reads        Reads
	OrderedReads map[string][]string
}

func (r *ReadSet) add(ns core.Namespace, key string, block core.BlockNum, txnum core.TxNum) {
	nsMap, in := r.Reads[ns]
	if !in {
		nsMap = make(map[core.Namespace]txPosition)

		r.Reads[ns] = nsMap
		r.OrderedReads[ns] = make([]string, 0, 8)
	}

	nsMap[key] = txPosition{block, txnum}
	r.OrderedReads[ns] = append(r.OrderedReads[ns], key)
}

func (r *ReadSet) get(ns core.Namespace, key string) (core.BlockNum, core.TxNum, bool) {
	entry, in := r.Reads[ns][key]
	return entry.Block, entry.TxNum, in
}

func (r *ReadSet) getAt(ns core.Namespace, i int) (string, bool) {
	slice := r.OrderedReads[ns]
	if i < 0 || i > len(slice)-1 {
		return "", false
	}

	return slice[i], true
}

func (r *ReadSet) clear(ns core.Namespace) {
	r.Reads[ns] = map[string]txPosition{}
	r.OrderedReads[ns] = []string{}
}

func entriesEqual[T any](r, o map[string]T, compare func(T, T) bool, nss ...core.Namespace) error {
	rKeys := getKeys(r, nss...)
	sort.Strings(rKeys)
	oKeys := getKeys(o, nss...)
	sort.Strings(oKeys)

	if len(rKeys) != len(oKeys) {
		return errors.Errorf("number of writes do not match [%d]!=[%d], [%v]!=[%v]", len(r), len(o), rKeys, oKeys)
	}

	for _, rKey := range rKeys {
		oValue, ok := o[rKey]
		if !ok {
			return errors.Errorf("read not found [%s]", rKey)
		}
		rValue := r[rKey]
		if !compare(rValue, oValue) {
			return errors.Errorf("writes for [%s] do not match [%v]!=[%v]", rKey, rValue, oValue)
		}
	}
	return nil
}

func getKeys[V any](m map[core.Namespace]V, nss ...core.Namespace) []string {
	metaWriteNamespaces := utils.Keys(m)
	if len(nss) == 0 {
		return metaWriteNamespaces
	}
	return utils.Intersection(nss, metaWriteNamespaces)
}
