/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mocks

import (
	"context"
	"encoding/binary"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
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

func (qe MockQE) GetStateMetadata(context.Context, driver.Namespace, driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return qe.Metadata, blockTxIndexToBytes(1, 1), nil
}

func (qe MockQE) GetState(_ context.Context, _ driver.Namespace, pkey driver.PKey) (*driver.VersionedRead, error) {
	return &driver.VersionedRead{
		Key:     pkey,
		Raw:     qe.State.Raw,
		Version: qe.State.Version,
	}, nil
}

func (qe MockQE) Done() {
}

type MockTxStatusStore struct {
}

func (m MockTxStatusStore) GetTxStatus(_ context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	return &driver.TxStatus{TxID: txID, Code: 1}, nil
}

func blockTxIndexToBytes(Block driver.BlockNum, TxNum driver.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(Block))
	binary.BigEndian.PutUint32(buf[4:], uint32(TxNum))
	return buf
}
