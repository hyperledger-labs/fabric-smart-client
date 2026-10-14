/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// Vault models a container of states
type Vault interface {
	// GetState loads the state identified by the tuple [namespace, id] into the passed state reference.
	GetState(ctx context.Context, namespace driver.Namespace, id driver.PKey, state any) error

	// GetStateCertification returns a serialized transaction that reads and rewrites the value stored
	// at [namespace, key], endorsed with this node's default identity to attest that value.
	GetStateCertification(ctx context.Context, namespace driver.Namespace, key driver.PKey) ([]byte, error)
}

// VaultService models a vault instance provider
type VaultService interface {
	// Vault returns the world state for the passed channel.
	Vault(network, channel string) (Vault, error)
}
