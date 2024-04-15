/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type TxStatusChangeListener struct {
	ExpectedTxID string
	WG           *sync.WaitGroup
}

func NewTxStatusChangeListener(expectedTxID string, WG *sync.WaitGroup) *TxStatusChangeListener {
	return &TxStatusChangeListener{ExpectedTxID: expectedTxID, WG: WG}
}

func (t *TxStatusChangeListener) OnStatus(txID string, _ driver.ValidationCode, _ string) {
	if txID == t.ExpectedTxID {
		t.WG.Done()
	}
}
