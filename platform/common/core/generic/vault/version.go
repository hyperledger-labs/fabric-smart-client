/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"encoding/binary"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

var zeroVersion = []byte{0, 0, 0, 0, 0, 0, 0, 0}

type BlockTxIndexVersionComparator struct{}

func (*BlockTxIndexVersionComparator) Equal(a, c driver.RawVersion) bool {
	return Equal(a, c)
}

type BlockTxIndexVersionBuilder struct{}

func (*BlockTxIndexVersionBuilder) VersionedValues(_ *ReadWriteSet, _ driver.Namespace, writes NamespaceWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]driver.VaultValue, error) {
	vals := make(map[driver.PKey]driver.VaultValue, len(writes))
	for pkey, val := range writes {
		vals[pkey] = driver.VaultValue{Raw: val, Version: BlockTxIndexToBytes(block, indexInBloc)}
	}
	return vals, nil
}

func (*BlockTxIndexVersionBuilder) VersionedMetaValues(_ *ReadWriteSet, _ driver.Namespace, writes KeyedMetaWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]driver.VaultMetadataValue, error) {
	vals := make(map[driver.PKey]driver.VaultMetadataValue, len(writes))
	for pkey, val := range writes {
		vals[pkey] = driver.VaultMetadataValue{Metadata: val, Version: BlockTxIndexToBytes(block, indexInBloc)}
	}
	return vals, nil
}

type BlockTxIndexVersionMarshaller struct{}

func (BlockTxIndexVersionMarshaller) FromBytes(data Version) (driver.BlockNum, driver.TxNum, error) {
	if len(data) == 0 {
		return 0, 0, nil
	}
	if len(data) != 8 {
		return 0, 0, errors.Errorf("block number must be 8 bytes, but got %d", len(data))
	}
	Block := driver.BlockNum(binary.BigEndian.Uint32(data[:4]))
	TxNum := driver.TxNum(binary.BigEndian.Uint32(data[4:]))
	return Block, TxNum, nil
}

func (BlockTxIndexVersionMarshaller) ToBytes(bn driver.BlockNum, txn driver.TxNum) Version {
	return BlockTxIndexToBytes(bn, txn)
}

func BlockTxIndexToBytes(block driver.BlockNum, txNum driver.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(block))
	binary.BigEndian.PutUint32(buf[4:], uint32(txNum))
	return buf
}

func Equal(a, c driver.RawVersion) bool {
	if bytes.Equal(a, c) {
		return true
	}
	if len(a) == 0 && bytes.Equal(zeroVersion, c) {
		return true
	}
	if len(c) == 0 && bytes.Equal(zeroVersion, a) {
		return true
	}
	return false
}
