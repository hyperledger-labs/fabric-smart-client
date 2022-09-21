/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Finality interface {
	// IsFinal takes in input a transaction id and waits for its confirmation
	// with the respect to the passed context that can be used to set a deadline
	// for the waiting time.
	IsFinal(ctx context.Context, txID string) error

	// IsFinalForParties takes in input a transaction id and an array of identities.
	// The identities are contacted to gather information about the finality of the
	// passed transaction
	IsFinalForParties(txID string, parties ...view.Identity) error
}
