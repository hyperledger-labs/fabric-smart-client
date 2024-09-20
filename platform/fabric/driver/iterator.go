/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

type SeekStart struct{}

type SeekEnd struct{}

type SeekPos struct {
	Txid string
}

type SeekSet struct {
	TxIDs []string
}

type ByNum struct {
	TxID    string
	Code    ValidationCode
	Message string
}

type TxIDIterator = collections.Iterator[*driver.ByNum[ValidationCode]]

type TXIDStore interface {
	GetLastTxID() (string, error)
	Iterator(pos interface{}) (TxIDIterator, error)
	Get(txid string) (ValidationCode, string, error)
	Set(txID string, code ValidationCode, message string) error
	SetMultiple(txs []driver.ByNum[ValidationCode]) error
}
