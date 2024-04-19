/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("fabric.iou")

type FinalityListener struct {
	ExpectedTxID string
	ExpectedVC   fabric.ValidationCode
	WaitGroup    *sync.WaitGroup
}

func NewFinalityListener(expectedTxID string, expectedVC fabric.ValidationCode, waitGroup *sync.WaitGroup) *FinalityListener {
	return &FinalityListener{ExpectedTxID: expectedTxID, ExpectedVC: expectedVC, WaitGroup: waitGroup}
}

func (t *FinalityListener) OnStatus(txID core.TxID, vc fabric.ValidationCode, _ string) {
	logger.Infof("on status [%s][%d]", txID, vc)
	if txID == t.ExpectedTxID && vc == t.ExpectedVC {
		time.Sleep(5 * time.Second)
		t.WaitGroup.Done()
	}
	return
}
