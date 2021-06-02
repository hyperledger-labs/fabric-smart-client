/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

func (c *channel) IsFinal(txID string) error {
	return c.finality.IsFinal(txID)
}

func (c *channel) IsFinalForParties(txID string, parties ...view.Identity) error {
	return c.finality.IsFinalForParties(txID, parties...)
}
