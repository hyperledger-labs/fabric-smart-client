/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type FinalityListener struct {
	ExpectedTxID string
	WaitGroup    *sync.WaitGroup
}

func NewFinalityListener(expectedTxID string, WG *sync.WaitGroup) *FinalityListener {
	return &FinalityListener{ExpectedTxID: expectedTxID, WaitGroup: WG}
}

func (t *FinalityListener) OnStatus(txID string, _ driver.ValidationCode, _ string) {
	if txID == t.ExpectedTxID {
		t.WaitGroup.Done()
	}
}
