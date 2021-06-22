/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// NewOrderingAndFinalityView returns a view that does the following:
// 1. Sends the passed transaction to the ordering service.
// 2. Waits for the finality of the transaction.
func NewOrderingAndFinalityView(tx *Transaction) view.View {
	return endorser.NewOrderingAndFinalityView(tx.tx)
}
