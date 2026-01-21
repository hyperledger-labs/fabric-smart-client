/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/driver"
)

type (
	TxID             = driver.TxID
	FinalityListener = driver.FinalityListener
)

type Finality struct {
	manager driver.FinalityListenerManager
}

func NewFinality(manager driver.FinalityListenerManager) *Finality {
	return &Finality{manager: manager}
}

func (f *Finality) AddFinalityListener(txID TxID, listener FinalityListener) error {
	return f.manager.AddFinalityListener(txID, listener)
}

func (f *Finality) RemoveFinalityListener(txID TxID, listener FinalityListener) error {
	return f.manager.RemoveFinalityListener(txID, listener)
}
