/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
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

func (i *Inspector) GetState(namespace driver.Namespace, key driver.PKey) (driver.RawValue, error) {
	return i.rws.writeSet.get(namespace, key), nil
}

func (i *Inspector) GetStateMetadata(namespace driver.Namespace, key driver.PKey) (driver.Metadata, error) {
	return i.rws.metaWriteSet.get(namespace, key), nil
}

func (i *Inspector) GetReadKeyAt(ns driver.Namespace, pos int) (string, error) {
	key, in := i.rws.readSet.getAt(ns, pos)
	if !in {
		return "", errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}
	return key, nil
}

func (i *Inspector) GetReadAt(ns driver.Namespace, pos int) (driver.PKey, driver.RawValue, error) {
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

func (i *Inspector) GetWriteAt(ns driver.Namespace, pos int) (driver.PKey, driver.RawValue, error) {

	key, in := i.rws.writeSet.getAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no write at position %d for namespace %s", pos, ns)
	}

	return key, i.rws.writeSet.get(ns, key), nil
}

func (i *Inspector) NumReads(ns driver.Namespace) int {
	return len(i.rws.reads[ns])
}

func (i *Inspector) NumWrites(ns driver.Namespace) int {
	return len(i.rws.writes[ns])
}

func (i *Inspector) KeyExist(ns driver.Namespace, key driver.PKey) (bool, error) {
	for pos := 0; pos < i.NumReads(ns); pos++ {
		k, err := i.GetReadKeyAt(ns, pos)
		if err != nil {
			return false, errors.WithMessagef(err, "Error reading key at [%d]", pos)
		}
		if strings.Contains(k, key) {
			return true, nil
		}
	}
	return false, nil
}

func (i *Inspector) Namespaces() []driver.Namespace {
	mergedMaps := map[driver.Namespace]struct{}{}

	for ns := range i.rws.reads {
		mergedMaps[ns] = struct{}{}
	}
	for ns := range i.rws.writes {
		mergedMaps[ns] = struct{}{}
	}

	namespaces := make([]driver.Namespace, 0, len(mergedMaps))
	for ns := range mergedMaps {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}
