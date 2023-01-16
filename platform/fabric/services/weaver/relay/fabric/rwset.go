/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"bytes"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
	"github.com/hyperledger/fabric-protos-go/ledger/rwset"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/pkg/errors"
)

type readWriteSet struct {
	readSet
	writeSet
	metaWriteSet
}

func (rws *readWriteSet) populate(rwsetBytes []byte, txid string) error {
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

		for _, read := range nsrws.KvRwSet.Reads {
			bn := uint64(0)
			txn := uint64(0)
			if read.Version != nil {
				bn = read.Version.BlockNum
				txn = read.Version.TxNum
			}
			rws.readSet.add(ns, read.Key, bn, txn)
		}

		for _, write := range nsrws.KvRwSet.Writes {
			if err := rws.writeSet.add(ns, write.Key, write.Value); err != nil {
				return err
			}
		}

		for _, metaWrite := range nsrws.KvRwSet.MetadataWrites {
			metadata := map[string][]byte{}
			for _, entry := range metaWrite.Entries {
				metadata[entry.Name] = append([]byte(nil), entry.Value...)
			}

			if err := rws.metaWriteSet.add(ns, metaWrite.Key, metadata); err != nil {
				return err
			}
		}
	}

	return nil
}

type metaWrites map[string][]byte

func (r metaWrites) Equals(o metaWrites) error {
	if len(r) != len(o) {
		return errors.Errorf("number of meta writes do not match [%v]!=[%v]", len(r), len(o))
	}

	for k, v := range r {
		v2, ok := o[k]
		if !ok {
			return errors.Errorf("read not found [%s]", k)
		}
		if !bytes.Equal(v, v2) {
			return errors.Errorf("writes for [%s] do not match [%v]!=[%v]", k, v, v2)
		}
	}

	return nil
}

type keyedMetaWrites map[string]metaWrites

func (r keyedMetaWrites) Equals(o keyedMetaWrites) error {
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

func (r keyedMetaWrites) keys() []string {
	var res []string
	for k := range r {
		res = append(res, k)
	}
	return res
}

type namespaceKeyedMetaWrites map[string]keyedMetaWrites

type metaWriteSet struct {
	metawrites namespaceKeyedMetaWrites
}

func (w *metaWriteSet) add(ns, key string, meta map[string][]byte) error {
	if err := keys.ValidateNs(ns); err != nil {
		return err
	}

	nsMap, in := w.metawrites[ns]
	if !in {
		nsMap = keyedMetaWrites{}
		w.metawrites[ns] = nsMap
	}

	metadata := map[string][]byte{}
	for k, v := range meta {
		metadata[k] = append([]byte(nil), v...)
	}

	nsMap[key] = metadata

	return nil
}

func (w *metaWriteSet) get(ns, key string) map[string][]byte {
	return w.metawrites[ns][key]
}

type namespaceWrites map[string][]byte

func (r namespaceWrites) Keys() []string {
	var keys []string
	for k := range r {
		keys = append(keys, k)
	}
	return keys
}

func (r namespaceWrites) Equals(o namespaceWrites) error {
	if len(r) != len(o) {
		return errors.Errorf("number of writes do not match [%d]!=[%d], [%v]!=[%v]", len(r), len(o), r.Keys(), o.Keys())
	}

	for k, v := range r {
		v2, ok := o[k]
		if !ok {
			return errors.Errorf("read not found [%s]", k)
		}
		if !bytes.Equal(v, v2) {
			return errors.Errorf("writes for [%s] do not match [%v]!=[%v]", k, v, v2)
		}
	}

	return nil
}

type writes map[string]namespaceWrites

type writeSet struct {
	writes        writes
	orderedWrites map[string][]string
}

func (w *writeSet) add(ns, key string, value []byte) error {
	if err := keys.ValidateNs(ns); err != nil {
		return err
	}

	nsMap, in := w.writes[ns]
	if !in {
		nsMap = map[string][]byte{}

		w.writes[ns] = nsMap
		w.orderedWrites[ns] = make([]string, 0, 8)
	}

	nsMap[key] = append([]byte(nil), value...)
	w.orderedWrites[ns] = append(w.orderedWrites[ns], key)

	return nil
}

func (w *writeSet) get(ns, key string) []byte {
	return w.writes[ns][key]
}

func (w *writeSet) getAt(ns string, i int) (key string, in bool) {
	slice := w.orderedWrites[ns]
	if i < 0 || i > len(slice)-1 {
		return "", false
	}

	return slice[i], true
}

type namespaceReads map[string]struct {
	block uint64
	txnum uint64
}

func (r namespaceReads) Equals(o namespaceReads) error {
	if len(r) != len(o) {
		return errors.Errorf("number of reads do not match [%v]!=[%v]", len(r), len(o))
	}

	for k, v := range r {
		v2, ok := o[k]
		if !ok {
			return errors.Errorf("read not found [%s]", k)
		}
		if v.block != v2.block || v.txnum != v2.txnum {
			return errors.Errorf("reads for [%s] do not match [%d,%d]!=[%d,%d]", k, v.block, v.txnum, v2.block, v2.txnum)
		}
	}

	return nil
}

type reads map[string]namespaceReads

type readSet struct {
	reads        reads
	orderedReads map[string][]string
}

func (r *readSet) add(ns, key string, block, txnum uint64) {
	nsMap, in := r.reads[ns]
	if !in {
		nsMap = make(map[string]struct {
			block uint64
			txnum uint64
		})

		r.reads[ns] = nsMap
		r.orderedReads[ns] = make([]string, 0, 8)
	}

	nsMap[key] = struct {
		block uint64
		txnum uint64
	}{block, txnum}
	r.orderedReads[ns] = append(r.orderedReads[ns], key)
}

func (r *readSet) getAt(ns string, i int) (key string, in bool) {
	slice := r.orderedReads[ns]
	if i < 0 || i > len(slice)-1 {
		return "", false
	}

	return slice[i], true
}
