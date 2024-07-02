/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/etx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// NewOrderingAndFinalityView returns a view that does the following:
// 1. Sends the passed transaction to the ordering service.
// 2. Waits for the finality of the transaction.
func NewOrderingAndFinalityView(tx *Transaction) view.View {
	return etx.NewOrderingAndFinalityView(tx.tx)
}

func NewOrderingAndFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) view.View {
	return etx.NewOrderingAndFinalityWithTimeoutView(tx.tx, timeout)
}
