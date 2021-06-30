/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// NewFinalityView returns a new instance of the finality view that waits for the finality of the passed transaction.
func NewFinalityView(tx *Transaction) view.View {
	return endorser.NewFinalityView(tx.tx)
}
