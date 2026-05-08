/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

var logger = logging.MustGetLogger()

// FinalityListener bridges the Fabric-X-Committer's per-transaction status
// stream into a sync.WaitGroup that the calling view can block on.
//
// Migration note: in classical chaincode there is no equivalent — the chaincode
// returns a value once endorsement is collected, and the client polls a peer
// for finality. In FSC-on-Fabric-X the view itself is the place where
// post-finality logic runs, so we attach a listener and wait.
type FinalityListener struct {
	ExpectedTxID string
	ExpectedVC   fdriver.ValidationCode
	WaitGroup    *sync.WaitGroup
}

func NewFinalityListener(expectedTxID string, expectedVC fdriver.ValidationCode, wg *sync.WaitGroup) *FinalityListener {
	return &FinalityListener{ExpectedTxID: expectedTxID, ExpectedVC: expectedVC, WaitGroup: wg}
}

func (f *FinalityListener) OnStatus(_ context.Context, txID driver.TxID, vc fdriver.ValidationCode, _ string) {
	logger.Infof("finality listener: tx=%s vc=%d", txID, vc)
	if txID == f.ExpectedTxID && vc == f.ExpectedVC {
		f.WaitGroup.Done()
	}
}
