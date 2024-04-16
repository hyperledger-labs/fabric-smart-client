/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

type ValidationCode interface {
	comparable
}

type ValidationCodeProvider[V ValidationCode] interface {
	IsValid(V) bool
	ToInt32(V) int32
	FromInt32(int32) V
	Unknown() V
}

type SeekStart struct{}

type SeekEnd struct{}

type SeekPos struct {
	Txid string
}

type SeekSet struct {
	TxIDs []string
}

type ByNum[V comparable] struct {
	TxID    string
	Code    V
	Message string
}

type TxIDIterator[V comparable] interface {
	Next() (*ByNum[V], error)
	Close()
}

type TXIDStore[V comparable] interface {
	GetLastTxID() (string, error)
	Iterator(pos interface{}) (TxIDIterator[V], error)
}
