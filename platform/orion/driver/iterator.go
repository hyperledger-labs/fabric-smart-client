/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"

type SeekStart struct{}
type SeekEnd struct{}
type SeekPos struct {
	Txid string
}

type ByNum struct {
	Txid string
	Code ValidationCode
}

type TxidIterator = collections.Iterator[*ByNum]

type TXIDStore interface {
	GetLastTxID() (string, error)
	Iterator(pos interface{}) (TxidIterator, error)
}
