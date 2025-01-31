/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type CommonIteratorInterface interface {
	// HasNext returns true if the range query iterator contains additional keys
	// and values.
	HasNext() bool

	// Close closes the iterator. This should be called when done
	// reading from the iterator to free up resources.
	Close() error
}

// QueryIteratorInterface models a state iterator
type QueryIteratorInterface interface {
	CommonIteratorInterface

	Next(state interface{}) (string, error)
}

// Vault models a container of states
type Vault interface {
	// GetState loads the state identified by the tuple [namespace, id] into the passed state reference.
	GetState(ctx context.Context, namespace driver.Namespace, id driver.PKey, state interface{}) error

	GetStateCertification(ctx context.Context, namespace driver.Namespace, key driver.PKey) ([]byte, error)

	GetStateByPartialCompositeID(ctx context.Context, ns driver.Namespace, prefix string, attrs []string) (QueryIteratorInterface, error)
}

// VaultService models a vault instance provider
type VaultService interface {
	// Vault returns the world state for the passed channel.
	Vault(network string, channel string) (Vault, error)
}
