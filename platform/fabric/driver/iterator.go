/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type SeekStart struct{}
type SeekEnd struct{}
type SeekPos struct {
	Txid string
}

type ByNum struct {
	Txid string
	Code ValidationCode
}

type TxidIterator interface {
	Next() (*ByNum, error)
	Close()
}

type TXIDStore interface {
	GetLastTxID() (string, error)
	Iterator(pos interface{}) (TxidIterator, error)
}
