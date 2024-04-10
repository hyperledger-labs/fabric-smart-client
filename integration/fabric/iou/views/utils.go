/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import "sync"

type TxStatusChangeListener struct {
	ExpectedTxID string
	WG           *sync.WaitGroup
}

func NewTxStatusChangeListener(expectedTxID string, WG *sync.WaitGroup) *TxStatusChangeListener {
	return &TxStatusChangeListener{ExpectedTxID: expectedTxID, WG: WG}
}

func (t *TxStatusChangeListener) OnStatusChange(txID string, status int, statusMessage string) error {
	if txID == t.ExpectedTxID {
		t.WG.Done()
	}
	return nil
}
