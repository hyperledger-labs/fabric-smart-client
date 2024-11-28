/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	"encoding/binary"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

type MockQE struct {
	State    driver.VersionedValue
	Metadata map[string][]byte
}

func NewMockQE() MockQE {
	return MockQE{
		State: driver.VersionedValue{
			Raw:     []byte("raw"),
			Version: blockTxIndexToBytes(1, 1),
		},
		Metadata: map[string][]byte{
			"md": []byte("meta"),
		},
	}
}

func (qe MockQE) GetStateMetadata(driver.Namespace, driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return qe.Metadata, blockTxIndexToBytes(1, 1), nil
}

func (qe MockQE) GetState(driver.Namespace, driver.PKey) (driver.VersionedValue, error) {
	return qe.State, nil
}

func (qe MockQE) Done() {
}

type MockTXIDStoreReader struct {
}

func (m MockTXIDStoreReader) Iterator(interface{}) (collections.Iterator[*driver.ByNum[int]], error) {
	panic("not implemented")
}
func (m MockTXIDStoreReader) Get(txID driver.TxID) (int, string, error) {
	return 1, txID, nil
}

func blockTxIndexToBytes(Block driver.BlockNum, TxNum driver.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(Block))
	binary.BigEndian.PutUint32(buf[4:], uint32(TxNum))
	return buf
}
