/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"golang.org/x/net/context"
)

type Finality interface {
	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(ctx context.Context, txID string) error

	// IsFinalForParties takes in input a transaction id and an array of identities.
	// The identities are contacted to gather information about the finality of the
	// passed transaction
	IsFinalForParties(txID string, parties ...view.Identity) error
}
