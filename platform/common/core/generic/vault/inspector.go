/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type Inspector struct {
	Rws ReadWriteSet
}

func (i *Inspector) IsValid() error {
	return nil
}

func (i *Inspector) IsClosed() bool {
	return false
}

func (i *Inspector) SetState(driver.Namespace, driver.PKey, driver.RawValue) error {
	panic("programming error: the rwset inspector is read-only")
}

func (i *Inspector) AddReadAt(ns driver.Namespace, key string, version Version) error {
	panic("programming error: the rwset inspector is read-only")
}

func (i *Inspector) GetState(namespace driver.Namespace, key driver.PKey, _ ...driver.GetStateOpt) (driver.RawValue, error) {
	return i.Rws.WriteSet.Get(namespace, key), nil
}

func (i *Inspector) GetDirectState(driver.Namespace, driver.PKey) ([]byte, error) {
	panic("programming error: no access to query executor")
}

func (i *Inspector) DeleteState(driver.Namespace, driver.PKey) error {
	panic("programming error: the rwset inspector is read-only")
}

func (i *Inspector) GetStateMetadata(namespace driver.Namespace, key driver.PKey, _ ...driver.GetStateOpt) (driver.Metadata, error) {
	return i.Rws.MetaWriteSet.Get(namespace, key), nil
}

func (i *Inspector) SetStateMetadata(driver.Namespace, driver.PKey, driver.Metadata) error {
	panic("programming error: the rwset inspector is read-only")
}

func (i *Inspector) SetStateMetadatas(ns driver.Namespace, kvs map[driver.PKey]driver.VaultMetadataValue) map[driver.PKey]error {
	panic("programming error: the rwset inspector is read-only")
}

func (i *Inspector) GetReadKeyAt(ns driver.Namespace, pos int) (driver.PKey, error) {
	key, in := i.Rws.ReadSet.GetAt(ns, pos)
	if !in {
		return "", errors.Errorf("no read at position %d for namespace %s", pos, ns)
	}
	return key, nil
}

func (i *Inspector) GetReadAt(ns driver.Namespace, pos int) (driver.PKey, driver.RawValue, error) {
	key, in := i.Rws.ReadSet.GetAt(ns, pos)
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

	key, in := i.Rws.WriteSet.GetAt(ns, pos)
	if !in {
		return "", nil, errors.Errorf("no write at position %d for namespace %s", pos, ns)
	}

	return key, i.Rws.WriteSet.Get(ns, key), nil
}

func (i *Inspector) NumReads(ns driver.Namespace) int {
	return len(i.Rws.Reads[ns])
}

func (i *Inspector) NumWrites(ns driver.Namespace) int {
	return len(i.Rws.Writes[ns])
}

func (i *Inspector) Namespaces() []driver.Namespace {
	mergedMaps := map[driver.Namespace]struct{}{}

	for ns := range i.Rws.Reads {
		mergedMaps[ns] = struct{}{}
	}
	for ns := range i.Rws.Writes {
		mergedMaps[ns] = struct{}{}
	}

	namespaces := make([]driver.Namespace, 0, len(mergedMaps))
	for ns := range mergedMaps {
		namespaces = append(namespaces, ns)
	}

	return namespaces
}

func (i *Inspector) AppendRWSet(driver.RawValue, ...driver.Namespace) error {
	panic("programming error: the rwset inspector is read-only")
}

func (i *Inspector) Bytes() ([]byte, error) {
	panic("programming error: unexpected call")
}

func (i *Inspector) Equals(other interface{}, nss ...driver.Namespace) error {
	panic("programming error: unexpected call")
}

func (i *Inspector) Done() {

}

func (i *Inspector) Clear(driver.Namespace) error {
	panic("programming error: unexpected call")
}
