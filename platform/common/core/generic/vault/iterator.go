/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/core"

type ValidationCode interface {
	comparable
}

type ValidationCodeProvider[V ValidationCode] interface {
	ToInt32(V) int32
	FromInt32(int32) V
	Unknown() V
	Busy() V
	Valid() V
	Invalid() V
}

type SeekStart struct{}

type SeekEnd struct{}

type SeekPos struct {
	Txid core.TxID
}

type SeekSet struct {
	TxIDs []core.TxID
}

type ByNum[V comparable] struct {
	TxID    core.TxID
	Code    V
	Message string
}

type TxIDIterator[V comparable] interface {
	Next() (*ByNum[V], error)
	Close()
}
