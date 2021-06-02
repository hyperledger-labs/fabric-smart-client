/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package iou

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type IOUState struct {
	Amount   uint            // Amount the borrower owes the lender
	LinearID string          // Unique identifier of this state
	Parties  []view.Identity // The list of owners of this state
}

func (i *IOUState) SetLinearID(id string) string {
	if len(i.LinearID) == 0 {
		i.LinearID = id
	}
	return i.LinearID
}

func (i *IOUState) Owners() state.Identities {
	return i.Parties
}
