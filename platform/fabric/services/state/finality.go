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

// NewFinalityView returns a new instance of the finality view that waits for the finality of the passed transaction.
func NewFinalityView(tx *Transaction) view.View {
	return etx.NewFinalityView(tx.tx)
}

// NewFinalityWithTimeoutView runs the finality view for the passed transaction and timeout
func NewFinalityWithTimeoutView(tx *Transaction, timeout time.Duration) view.View {
	return etx.NewFinalityWithTimeoutView(tx.tx, timeout)
}
