/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricx

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
)

type (
	TxID             = driver.TxID
	FinalityListener = fabric.FinalityListener
)

type Finality struct {
	manager finality.ListenerManager
}

func NewFinality(manager finality.ListenerManager) *Finality {
	return &Finality{manager: manager}
}

func (f *Finality) AddFinalityListener(txID TxID, listener FinalityListener) error {
	return f.manager.AddFinalityListener(txID, listener)
}

func (f *Finality) RemoveFinalityListener(txID TxID, listener FinalityListener) error {
	return f.manager.RemoveFinalityListener(txID, listener)
}
