/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

type committerListenerManager struct {
	committer *fabric.Committer
}

func NewCommitterFLM(committer *fabric.Committer) *committerListenerManager {
	return &committerListenerManager{committer: committer}
}

func (m *committerListenerManager) AddFinalityListener(_ driver.Namespace, txID driver.TxID, listener fabric.FinalityListener) error {
	return m.committer.AddFinalityListener(txID, listener)
}

func (m *committerListenerManager) RemoveFinalityListener(txID string, listener fabric.FinalityListener) error {
	return m.committer.RemoveFinalityListener(txID, listener)
}
