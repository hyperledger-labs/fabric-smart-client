/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"encoding/binary"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/pkg/errors"
)

type CounterBasedVersionBuilder struct{}

func (c *CounterBasedVersionBuilder) VersionedValues(rws *vault.ReadWriteSet, ns driver.Namespace, writes vault.NamespaceWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]vault.VersionedValue, error) {
	vals := make(map[driver.PKey]vault.VersionedValue, len(writes))
	reads := rws.Reads[ns]

	for pkey, val := range writes {
		v, err := version(reads, pkey)
		if err != nil {
			return nil, err
		}
		vals[pkey] = vault.VersionedValue{Raw: val, Version: v}
	}
	return vals, nil
}

func version(reads vault.NamespaceReads, pkey driver.PKey) (vault.Version, error) {
	// Search the corresponding read.
	v, ok := reads[pkey]
	if !ok {
		// this is a blind write, we should check the vault.
		// Let's assume here that a blind write always starts from version 0
		return Marshal(0), nil
	}

	// parse the version as an integer, then increment it
	counter, err := Unmarshal(v)
	if err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling version for %s:%v", pkey, v)
	}
	return Marshal(counter + 1), nil
}

func (c *CounterBasedVersionBuilder) VersionedMetaValues(rws *vault.ReadWriteSet, ns driver.Namespace, writes vault.KeyedMetaWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]driver.VersionedMetadataValue, error) {
	vals := make(map[driver.PKey]driver.VersionedMetadataValue, len(writes))
	reads := rws.Reads[ns]

	for pkey, val := range writes {
		v, err := version(reads, pkey)
		if err != nil {
			return nil, err
		}

		vals[pkey] = driver.VersionedMetadataValue{Metadata: val, Version: v}
	}
	return vals, nil
}

type CounterBasedVersionComparator struct{}

func (c *CounterBasedVersionComparator) Equal(v1, v2 driver.RawVersion) bool {
	return bytes.Equal(v1, v2)
}

func Marshal(v uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf[:4], v)
	return buf
}

func Unmarshal(raw []byte) (uint32, error) {
	if len(raw) != 4 {
		return 0, errors.Errorf("invalid version, expected 4 bytes, got [%d]", len(raw))
	}
	return binary.BigEndian.Uint32(raw), nil
}
