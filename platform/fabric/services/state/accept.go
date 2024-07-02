/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/etx"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func NewAcceptView(tx *Transaction) view.View {
	return etx.NewAcceptView(tx.tx)
}
