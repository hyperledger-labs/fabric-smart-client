/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"sort"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"

	"github.com/google/go-cmp/cmp"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
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

func (rws *ReadWriteSet) populate(rwsetBytes []byte, txid string, namespaces ...string) error {
	txRWSet := &rwset.TxReadWriteSet{}
	err := proto.Unmarshal(rwsetBytes, txRWSet)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes for txid %s, unmarshal failed", txid)
	}

	rwsIn, err := rwsetutil.TxRwSetFromProtoMsg(txRWSet)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes for txid %s, TxRwSetFromProtoMsg failed", txid)
	}

	for _, nsrws := range rwsIn.NsRwSets {
		ns := nsrws.NameSpace

		// skip if not in the list of namespaces
		if len(namespaces) > 0 {
			notFound := true
			for _, s := range namespaces {
				if s == ns {
					notFound = false
					break
				}
			}
			if notFound {
				continue
			}
		}

		for _, read := range nsrws.KvRwSet.Reads {
			bn := uint64(0)
			txn := uint64(0)
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
	if len(r) != len(o) {
		return errors.Errorf("number of meta writes do not match [%v]!=[%v]", len(r), len(o))
	}

	for k, v := range r {
		v2, ok := o[k]
		if !ok {
			return errors.Errorf("read not found [%s]", k)
		}
		if !bytes.Equal(v, v2) {
			return errors.Errorf("writes for [%s] do not match [%v]!=[%v]", k, hash.Hashable(v), hash.Hashable(v2))
		}
	}

	return nil
}

type KeyedMetaWrites map[string]MetaWrites

func (r KeyedMetaWrites) Equals(o KeyedMetaWrites) error {
	rKeys := r.keys()
	sort.Strings(rKeys)
	oKeys := o.keys()
	sort.Strings(oKeys)
	if diff := cmp.Diff(rKeys, oKeys); len(diff) != 0 {
		return errors.Errorf("namespaces do not match [%s]", diff)
	}

	for _, key := range rKeys {
		if err := r[key].Equals(o[key]); err != nil {
			return errors.Wrapf(err, "meta writes for key [%s] do not match", key)
		}
	}

	return nil
}

func (r KeyedMetaWrites) keys() []string {
	var res []string
	for k := range r {
		res = append(res, k)
	}
	return res
}

type NamespaceKeyedMetaWrites map[string]KeyedMetaWrites

func (r NamespaceKeyedMetaWrites) equals(o NamespaceKeyedMetaWrites, nss ...string) error {
	rKeys := r.keys(nss...)
	sort.Strings(rKeys)
	oKeys := o.keys(nss...)
	sort.Strings(oKeys)
	if diff := cmp.Diff(rKeys, oKeys); len(diff) != 0 {
		return errors.Errorf("namespaces do not match [%s]", diff)
	}

	for _, key := range rKeys {
		if err := r[key].
			Equals(o[key]); err != nil {
			return errors.Wrapf(err, "namespaces [%s] do not match", key)
		}
	}

	return nil
}

func (r NamespaceKeyedMetaWrites) keys(nss ...string) []string {
	var res []string
	for k := range r {
		if len(nss) == 0 {
			res = append(res, k)
			continue
		}

		for _, s := range nss {
			if s == k {
				res = append(res, k)
				break
			}
		}
	}
	return res
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
	var keys []string
	for k := range r {
		keys = append(keys, k)
	}
	return keys
}

func (r NamespaceWrites) Equals(o NamespaceWrites) error {
	if len(r) != len(o) {
		return errors.Errorf("number of writes do not match [%d]!=[%d], [%v]!=[%v]", len(r), len(o), r.Keys(), o.Keys())
	}

	for k, v := range r {
		v2, ok := o[k]
		if !ok {
			return errors.Errorf("read not found [%s]", k)
		}
		if !bytes.Equal(v, v2) {
			return errors.Errorf("writes for [%s] do not match [%v]!=[%v]", k, hash.Hashable(v), hash.Hashable(v2))
		}
	}

	return nil
}

type Writes map[string]NamespaceWrites

func (r Writes) equals(o Writes, nss ...string) error {
	rKeys := r.keys(nss...)
	sort.Strings(rKeys)
	oKeys := o.keys(nss...)
	sort.Strings(oKeys)
	if diff := cmp.Diff(rKeys, oKeys); len(diff) != 0 {
		return errors.Errorf("namespaces do not match [%s]", diff)
	}

	for _, key := range rKeys {
		if err := r[key].Equals(o[key]); err != nil {
			return errors.Wrapf(err, "namespaces [%s] do not match", key)
		}
	}

	return nil
}

func (r Writes) keys(nss ...string) []string {
	var res []string
	for k := range r {
		if len(nss) == 0 {
			res = append(res, k)
			continue
		}

		for _, s := range nss {
			if s == k {
				res = append(res, k)
				break
			}
		}
	}
	return res
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

type NamespaceReads map[string]struct {
	Block uint64
	TxNum uint64
}

func (r NamespaceReads) Equals(o NamespaceReads) error {
	if len(r) != len(o) {
		return errors.Errorf("number of reads do not match [%v]!=[%v]", len(r), len(o))
	}

	for k, v := range r {
		v2, ok := o[k]
		if !ok {
			return errors.Errorf("read not found [%s]", k)
		}
		if v.Block != v2.Block || v.TxNum != v2.TxNum {
			return errors.Errorf("reads for [%s] do not match [%d,%d]!=[%d,%d]", k, v.Block, v.TxNum, v2.Block, v2.TxNum)
		}
	}

	return nil
}

type Reads map[string]NamespaceReads

func (r Reads) equals(o Reads, nss ...string) error {
	rKeys := r.keys(nss...)
	sort.Strings(rKeys)
	oKeys := o.keys(nss...)
	sort.Strings(oKeys)
	if diff := cmp.Diff(rKeys, oKeys); len(diff) != 0 {
		return errors.Errorf("namespaces do not match [%s]", diff)
	}

	for _, key := range rKeys {
		if err := r[key].Equals(o[key]); err != nil {
			return errors.Wrapf(err, "namespaces [%s] do not match", key)
		}
	}

	return nil
}

func (r Reads) keys(nss ...string) []string {
	var res []string
	for k := range r {
		if len(nss) == 0 {
			res = append(res, k)
			continue
		}

		for _, s := range nss {
			if s == k {
				res = append(res, k)
				break
			}
		}
	}
	return res
}

type ReadSet struct {
	Reads        Reads
	OrderedReads map[string][]string
}

func (r *ReadSet) add(ns, key string, block, txnum uint64) {
	nsMap, in := r.Reads[ns]
	if !in {
		nsMap = make(map[string]struct {
			Block uint64
			TxNum uint64
		})

		r.Reads[ns] = nsMap
		r.OrderedReads[ns] = make([]string, 0, 8)
	}

	nsMap[key] = struct {
		Block uint64
		TxNum uint64
	}{block, txnum}
	r.OrderedReads[ns] = append(r.OrderedReads[ns], key)
}

func (r *ReadSet) get(ns, key string) (block, txnum uint64, in bool) {
	entry, in := r.Reads[ns][key]
	block = entry.Block
	txnum = entry.TxNum
	return
}

func (r *ReadSet) getAt(ns string, i int) (key string, in bool) {
	slice := r.OrderedReads[ns]
	if i < 0 || i > len(slice)-1 {
		return "", false
	}

	return slice[i], true
}

func (r *ReadSet) clear(ns string) {
	r.Reads[ns] = map[string]struct {
		Block uint64
		TxNum uint64
	}{}
	r.OrderedReads[ns] = []string{}
}
