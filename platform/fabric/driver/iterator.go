/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"

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

type TxIDIterator = vault.TxIDIterator[ValidationCode]

type TXIDStore interface {
	GetLastTxID() (string, error)
	Iterator(pos interface{}) (TxIDIterator, error)
}
