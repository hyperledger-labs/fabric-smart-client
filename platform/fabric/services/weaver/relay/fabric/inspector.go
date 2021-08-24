/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/pkg/errors"
)

type Inspector struct {
	raw []byte
	rws readWriteSet
}

func newInspector() *Inspector {
	return &Inspector{
		rws: readWriteSet{
			readSet: readSet{
				orderedReads: map[string][]string{},
				reads:        reads{},
			},
			writeSet: writeSet{
				orderedWrites: map[string][]string{},
				writes:        writes{},
			},
			metaWriteSet: metaWriteSet{
				metawrites: namespaceKeyedMetaWrites{},
			},
		},
	}
}

func (i *Inspector) Byte() ([]byte, error) {
	return i.raw, nil
}

func (i *Inspector) IsValid() error {
	return nil
}

func (i *Inspector) GetState(namespace string, key string) ([]byte, error) {
	return i.rws.writeSet.get(namespace, key), nil
}

func (i *Inspector) GetStateMetadata(namespace, key string) (map[string][]byte, error) {
	return i.rws.metaWriteSet.get(namespace, key), nil
}

func (i *Inspector) GetReadKeyAt(ns string, pos int) (string, error) {
	key, in := i.rws.readSet.getAt(ns, pos)
	if !in {
		return "", errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}
	return key, nil
}

func (i *Inspector) GetReadAt(ns string, pos int) (string, []byte, error) {
	key, in := i.rws.readSet.getAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}

	val, err := i.GetState(ns, key)
	if err != nil {
		return "", nil, err
	}

	return key, val, nil
}

func (i *Inspector) GetWriteAt(ns string, pos int) (string, []byte, error) {

	key, in := i.rws.writeSet.getAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no write at position %d for namespace %s", pos, ns)
	}

	return key, i.rws.writeSet.get(ns, key), nil
}

func (i *Inspector) NumReads(ns string) int {
	return len(i.rws.reads[ns])
}

func (i *Inspector) NumWrites(ns string) int {
	return len(i.rws.writes[ns])
}

func (i *Inspector) Namespaces() []string {
	mergedMaps := map[string]struct{}{}

	for ns := range i.rws.reads {
		mergedMaps[ns] = struct{}{}
	}
	for ns := range i.rws.writes {
		mergedMaps[ns] = struct{}{}
	}

	namespaces := make([]string, 0, len(mergedMaps))
	for ns := range mergedMaps {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}
