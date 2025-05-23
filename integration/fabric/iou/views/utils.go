/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

var logger = logging.MustGetLogger()

type FinalityListener struct {
	ExpectedTxID string
	ExpectedVC   fdriver.ValidationCode
	WaitGroup    *sync.WaitGroup
}

func NewFinalityListener(expectedTxID string, expectedVC fdriver.ValidationCode, waitGroup *sync.WaitGroup) *FinalityListener {
	return &FinalityListener{ExpectedTxID: expectedTxID, ExpectedVC: expectedVC, WaitGroup: waitGroup}
}

func (t *FinalityListener) OnStatus(_ context.Context, txID driver.TxID, vc fdriver.ValidationCode, _ string) {
	logger.Infof("on status [%s][%d]", txID, vc)
	if txID == t.ExpectedTxID && vc == t.ExpectedVC {
		time.Sleep(5 * time.Second)
		t.WaitGroup.Done()
	}
}
