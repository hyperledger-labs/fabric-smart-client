/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package state

type CommonIteratorInterface interface {
	// HasNext returns true if the range query iterator contains additional keys
	// and values.
	HasNext() bool

	// Close closes the iterator. This should be called when done
	// reading from the iterator to free up resources.
	Close() error
}

// StateQueryIteratorInterface models a state iterator
type StateQueryIteratorInterface interface {
	CommonIteratorInterface

	Next(state interface{}) error
}

// WorldState models the world state
type WorldState interface {
	// GetState loads the state identified by the tuple [namespace, id] into the passed state reference.
	GetState(namespace string, id string, state interface{}) error

	GetStateCertification(namespace string, key string) ([]byte, error)

	GetStateByPartialCompositeID(ns string, prefix string, attrs []string) (StateQueryIteratorInterface, error)
}

// WorldStateService models the world state
type WorldStateService interface {
	// GetWorldState returns the world state for the passed channel.
	GetWorldState(network string, channel string) (WorldState, error)
}
