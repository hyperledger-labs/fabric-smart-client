/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// minAmount is the smallest amount an IOU can be created with, as in the Fabric IOU.
const minAmount = 5

// ValidateCreate checks c against the IOU rules the contract also enforces: existing, the
// on-chain state at c.ID, must not exist, the amount must be at least 5, and borrower and lender
// must differ.
func ValidateCreate(c contract.Create, existing contract.IOU) error {
	if existing.Version != 0 {
		return errors.Errorf("iou [%x] already exists", c.ID)
	}
	if c.Amount < minAmount {
		return errors.Errorf("invalid amount, expected at least %d, was [%d]", minAmount, c.Amount)
	}
	if c.Borrower == c.Lender {
		return errors.Errorf("borrower and lender must differ, both are [%s]", c.Borrower)
	}

	return nil
}

// ValidateUpdate checks u against stored, the on-chain state at u.ID: the IOU must exist, the new
// amount must be lower than the stored one, and u.Version must equal the stored version.
func ValidateUpdate(stored contract.IOU, u contract.Update) error {
	if stored.Version == 0 {
		return errors.Errorf("iou [%x] does not exist", u.ID)
	}
	if u.Amount >= stored.Amount {
		return errors.Errorf("invalid amount, [%d] expected to be less than [%d]", u.Amount, stored.Amount)
	}
	if u.Version != stored.Version {
		return errors.Errorf("invalid version, [%d] expected to be [%d]", u.Version, stored.Version)
	}

	return nil
}
