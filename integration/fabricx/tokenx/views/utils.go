/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// FinalityListener waits for a transaction to reach a specific validation status
type FinalityListener struct {
	ExpectedTxID string
	ExpectedVC   fdriver.ValidationCode
	WaitGroup    *sync.WaitGroup
}

// NewFinalityListener creates a new FinalityListener
func NewFinalityListener(expectedTxID string, expectedVC fdriver.ValidationCode, waitGroup *sync.WaitGroup) *FinalityListener {
	return &FinalityListener{
		ExpectedTxID: expectedTxID,
		ExpectedVC:   expectedVC,
		WaitGroup:    waitGroup,
	}
}

// OnStatus is called when a transaction status update is received
func (f *FinalityListener) OnStatus(_ context.Context, txID driver.TxID, vc fdriver.ValidationCode, message string) {
	if txID == f.ExpectedTxID && vc == f.ExpectedVC {
		time.Sleep(5 * time.Second) // Short delay to ensure state is committed
		f.WaitGroup.Done()
	}
}

// FinalityTimeout is the default timeout for waiting on transaction finality
const FinalityTimeout = 60 * time.Second
