/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"sort"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/keys"
)

type ReadWriteSet struct {
	ReadSet
	WriteSet
	MetaWriteSet
}

type MetaWrites = driver.Metadata

type KeyedMetaWrites map[string]MetaWrites

func (r KeyedMetaWrites) Equals(o KeyedMetaWrites) error {
	return entriesEqual(r, o, func(a, b MetaWrites) bool { return entriesEqual(a, b, bytes.Equal) == nil })
}

type NamespaceKeyedMetaWrites map[driver.Namespace]KeyedMetaWrites

func (r NamespaceKeyedMetaWrites) Equals(o NamespaceKeyedMetaWrites, nss ...string) error {
	return entriesEqual(r, o, func(a, b KeyedMetaWrites) bool { return a.Equals(b) == nil }, nss...)
}

type MetaWriteSet struct {
	MetaWrites NamespaceKeyedMetaWrites
}

func (w *MetaWriteSet) Add(ns, key string, meta map[string][]byte) error {
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

func (w *MetaWriteSet) Get(ns, key string) map[string][]byte {
	return w.MetaWrites[ns][key]
}

func (w *MetaWriteSet) In(ns, key string) bool {
	_, in := w.MetaWrites[ns][key]
	return in
}

func (w *MetaWriteSet) Clear(ns string) {
	w.MetaWrites[ns] = KeyedMetaWrites{}
}

type NamespaceWrites map[driver.PKey]driver.RawValue

func (r NamespaceWrites) Keys() []string {
	return collections.Keys(r)
}

func (r NamespaceWrites) Equals(o NamespaceWrites) error {
	return entriesEqual(r, o, bytes.Equal)
}

type Writes map[driver.Namespace]NamespaceWrites

func (r Writes) Equals(o Writes, nss ...string) error {
	return entriesEqual(r, o, func(a, b NamespaceWrites) bool { return a.Equals(b) == nil }, nss...)
}

type WriteSet struct {
	Writes        Writes
	OrderedWrites map[string][]string
}

func (w *WriteSet) Add(ns, key string, value []byte) error {
	if err := keys.ValidateNs(ns); err != nil {
		return err
	}

	nsMap, in := w.Writes[ns]
	if !in {
		nsMap = map[string][]byte{}

		w.Writes[ns] = nsMap
		w.OrderedWrites[ns] = make([]string, 0, 8)
	}

	_, exists := nsMap[key]
	nsMap[key] = append([]byte(nil), value...)
	if !exists {
		w.OrderedWrites[ns] = append(w.OrderedWrites[ns], key)
	}

	return nil
}

func (w *WriteSet) Get(ns, key string) []byte {
	return w.Writes[ns][key]
}

func (w *WriteSet) GetAt(ns string, i int) (key string, in bool) {
	slice := w.OrderedWrites[ns]
	if i < 0 || i > len(slice)-1 {
		return "", false
	}

	return slice[i], true
}

func (w *WriteSet) In(ns, key string) bool {
	_, in := w.Writes[ns][key]
	return in
}

func (w *WriteSet) Clear(ns string) {
	w.Writes[ns] = map[string][]byte{}
	w.OrderedWrites[ns] = []string{}
}

type Version = driver.RawVersion

type NamespaceReads map[string]Version

func (r NamespaceReads) Equals(o NamespaceReads) error {
	return entriesEqual(r, o, bytes.Equal)
}

type Reads map[driver.Namespace]NamespaceReads

func (r Reads) Equals(o Reads, nss ...driver.Namespace) error {
	return entriesEqual(r, o, func(a, b NamespaceReads) bool { return a.Equals(b) == nil }, nss...)
}

type ReadSet struct {
	Reads        Reads
	OrderedReads map[string][]string
}

func (r *ReadSet) Add(ns driver.Namespace, key string, version Version) {
	nsMap, in := r.Reads[ns]
	if !in {
		nsMap = make(map[driver.Namespace]Version)

		r.Reads[ns] = nsMap
		r.OrderedReads[ns] = make([]string, 0, 8)
	}

	nsMap[key] = version
	r.OrderedReads[ns] = append(r.OrderedReads[ns], key)
}

func (r *ReadSet) Get(ns driver.Namespace, key string) (Version, bool) {
	entry, in := r.Reads[ns][key]
	return entry, in
}

func (r *ReadSet) GetAt(ns driver.Namespace, i int) (string, bool) {
	slice := r.OrderedReads[ns]
	if i < 0 || i > len(slice)-1 {
		return "", false
	}

	return slice[i], true
}

func (r *ReadSet) Clear(ns driver.Namespace) {
	r.Reads[ns] = map[string]Version{}
	r.OrderedReads[ns] = []string{}
}

func entriesEqual[T any](r, o map[string]T, compare func(T, T) bool, nss ...driver.Namespace) error {
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

func getKeys[V any](m map[driver.Namespace]V, nss ...driver.Namespace) []string {
	metaWriteNamespaces := collections.Keys(m)
	if len(nss) == 0 {
		return metaWriteNamespaces
	}
	return collections.Intersection(nss, metaWriteNamespaces)
}
