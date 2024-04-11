/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"
)

type Finality interface {
	// IsFinal takes in input a transaction id and waits for its confirmation
	// with the respect to the passed context that can be used to set a deadline
	// for the waiting time.
	IsFinal(ctx context.Context, txID string) error
}
