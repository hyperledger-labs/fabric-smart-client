/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
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

type keyedMetaWrites map[string]metaWrites

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
