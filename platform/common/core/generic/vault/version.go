/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"bytes"
	"encoding/binary"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/pkg/errors"
)

var zeroVersion = []byte{0, 0, 0, 0, 0, 0, 0, 0}

type BlockTxIndexVersionComparator struct{}

func (b *BlockTxIndexVersionComparator) Equal(a, c driver2.RawVersion) bool {
	return Equal(a, c)
}

type BlockTxIndexVersionBuilder struct{}

func (b *BlockTxIndexVersionBuilder) VersionedValues(rws *ReadWriteSet, ns driver2.Namespace, writes NamespaceWrites, block driver2.BlockNum, indexInBloc driver2.TxNum) (map[driver2.PKey]VersionedValue, error) {
	vals := make(map[driver2.PKey]driver.VersionedValue, len(writes))
	for pkey, val := range writes {
		vals[pkey] = driver.VersionedValue{Raw: val, Version: BlockTxIndexToBytes(block, indexInBloc)}
	}
	return vals, nil
}

func (b *BlockTxIndexVersionBuilder) VersionedMetaValues(rws *ReadWriteSet, ns driver2.Namespace, writes KeyedMetaWrites, block driver2.BlockNum, indexInBloc driver2.TxNum) (map[driver2.PKey]driver2.VersionedMetadataValue, error) {
	vals := make(map[driver2.PKey]driver2.VersionedMetadataValue, len(writes))
	for pkey, val := range writes {
		vals[pkey] = driver2.VersionedMetadataValue{Metadata: val, Version: BlockTxIndexToBytes(block, indexInBloc)}
	}
	return vals, nil
}

type BlockTxIndexVersionMarshaller struct{}

func (m BlockTxIndexVersionMarshaller) FromBytes(data Version) (driver2.BlockNum, driver2.TxNum, error) {
	if len(data) == 0 {
		return 0, 0, nil
	}
	if len(data) != 8 {
		return 0, 0, errors.Errorf("block number must be 8 bytes, but got %d", len(data))
	}
	Block := driver2.BlockNum(binary.BigEndian.Uint32(data[:4]))
	TxNum := driver2.TxNum(binary.BigEndian.Uint32(data[4:]))
	return Block, TxNum, nil

}

func (m BlockTxIndexVersionMarshaller) ToBytes(bn driver2.BlockNum, txn driver2.TxNum) Version {
	return BlockTxIndexToBytes(bn, txn)
}

func BlockTxIndexToBytes(Block driver2.BlockNum, TxNum driver2.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(Block))
	binary.BigEndian.PutUint32(buf[4:], uint32(TxNum))
	return buf
}

func Equal(a, c driver2.RawVersion) bool {
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
