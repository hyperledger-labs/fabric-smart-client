/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Vault models a key-value store that can be updated by committing rwsets
type Vault interface {
	CommitTX(txid string, block uint64, indexInBloc int) error
	DiscardTx(txid string) error
}

type Committer interface {
	Status(txid string) (driver.ValidationCode, []string, []view.Identity, error)
	Validate(txid string) (driver.ValidationCode, error)
	CommitTX(txid string, block uint64, indexInBloc int) error
	DiscardTX(txid string) error
}

// Driver is the interface that must be implemented by a committer
// driver.
type Driver interface {
	// Open returns a new Committer with the respect to the passed vault.
	// The name is a string in a driver-specific format.
	// The returned Committer is only used by one goroutine at a time.
	Open(name string, sp view2.ServiceProvider, vault Vault) (Committer, error)
}
