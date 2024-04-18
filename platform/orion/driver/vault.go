/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type TxValidationStatus = driver2.TxValidationStatus[ValidationCode]

// Vault models a key value store that can be updated by committing rwsets
type Vault interface {
	driver2.Vault[ValidationCode]
	GetLastTxID() (string, error)
}
