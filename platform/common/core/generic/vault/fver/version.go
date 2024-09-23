/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fver

import (
	"bytes"
	"encoding/binary"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

var zeroVersion = []byte{0, 0, 0, 0, 0, 0, 0, 0}

func IsEqual(a, b driver.RawVersion) bool {
	if bytes.Equal(a, b) {
		return true
	}
	if len(a) == 0 && bytes.Equal(zeroVersion, b) {
		return true
	}
	if len(b) == 0 && bytes.Equal(zeroVersion, a) {
		return true
	}
	return false
}

func ToBytes(Block driver.BlockNum, TxNum driver.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(Block))
	binary.BigEndian.PutUint32(buf[4:], uint32(TxNum))
	return buf
}

func FromBytes(data []byte) (driver.BlockNum, driver.TxNum, error) {
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
