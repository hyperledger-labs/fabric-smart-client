/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"google.golang.org/protobuf/encoding/protowire"
)

type CounterBasedVersionBuilder struct{}

func (*CounterBasedVersionBuilder) VersionedValues(rws *vault.ReadWriteSet, ns driver.Namespace, writes vault.NamespaceWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]driver.VaultValue, error) {
	vals := make(map[driver.PKey]driver.VaultValue, len(writes))
	reads := rws.Reads[ns]

	for pkey, val := range writes {
		vals[pkey] = driver.VaultValue{Raw: val, Version: version(reads, pkey)}
	}
	return vals, nil
}

func version(reads vault.NamespaceReads, pkey driver.PKey) vault.Version {
	// Search the corresponding read.
	v, ok := reads[pkey]
	if !ok {
		// this is a blind write, we should check the vault.
		// Let's assume here that a blind write always starts from version 0
		return Marshal(0)
	}

	if len(v) == 0 {
		return Marshal(0)
	}

	// parse the version as an integer, then increment it
	counter := Unmarshal(v)
	return Marshal(counter + 1)
}

func (c *CounterBasedVersionBuilder) VersionedMetaValues(rws *vault.ReadWriteSet, ns driver.Namespace, writes vault.KeyedMetaWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]driver.VaultMetadataValue, error) {
	vals := make(map[driver.PKey]driver.VaultMetadataValue, len(writes))
	reads := rws.Reads[ns]

	for pkey, val := range writes {
		vals[pkey] = driver.VaultMetadataValue{Metadata: val, Version: version(reads, pkey)}
	}
	return vals, nil
}

type CounterBasedVersionComparator struct{}

func (*CounterBasedVersionComparator) Equal(v1, v2 driver.RawVersion) bool {
	return bytes.Equal(v1, v2)
}

func Marshal(v uint64) []byte {
	return protowire.AppendVarint(nil, v)
}

func Unmarshal(raw []byte) uint64 {
	v, _ := protowire.ConsumeVarint(raw)
	return v
}
