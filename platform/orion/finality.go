/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Finality struct {
	finality driver.Finality
}

func (c *Finality) IsFinal(txID string) error {
	return c.finality.IsFinal(txID)
}

func (c *Finality) IsFinalForParties(txID string, parties ...view.Identity) error {
	return c.finality.IsFinalForParties(txID, parties...)
}
