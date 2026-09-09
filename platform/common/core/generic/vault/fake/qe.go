/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import (
	"context"
	"encoding/binary"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type QE struct {
	State    driver.VaultValue
	Metadata map[string][]byte
}

func NewQE() QE {
	return QE{
		State: driver.VaultValue{
			Raw:     []byte("raw"),
			Version: blockTxIndexToBytes(1, 1),
		},
		Metadata: map[string][]byte{
			"md": []byte("meta"),
		},
	}
}

func (qe QE) GetStateMetadata(context.Context, driver.Namespace, driver.PKey) (driver.Metadata, driver.RawVersion, error) {
	return qe.Metadata, blockTxIndexToBytes(1, 1), nil
}

func (qe QE) GetState(_ context.Context, _ driver.Namespace, pkey driver.PKey) (*driver.VaultRead, error) {
	return &driver.VaultRead{
		Key:     pkey,
		Raw:     qe.State.Raw,
		Version: qe.State.Version,
	}, nil
}

func (QE) Done() error {
	return nil
}

type TxStatusStore struct{}

func (TxStatusStore) GetTxStatus(_ context.Context, txID driver.TxID) (*driver.TxStatus, error) {
	return &driver.TxStatus{TxID: txID, Code: 1}, nil
}

func blockTxIndexToBytes(block driver.BlockNum, txNum driver.TxNum) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], uint32(block))
	binary.BigEndian.PutUint32(buf[4:], uint32(txNum))
	return buf
}
