/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package states

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// IOU models the IOU state
type IOU struct {
	// Amount the borrower owes the lender
	Amount uint
	// Unique identifier of this state
	LinearID string
	// The list of owners of this state
	Parties []view.Identity
}

func (i *IOU) SetLinearID(id string) string {
	if len(i.LinearID) == 0 {
		i.LinearID = id
	}
	return i.LinearID
}

// Owners returns the list of identities owning this state
func (i *IOU) Owners() state.Identities {
	return i.Parties
}
