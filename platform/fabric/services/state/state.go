/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

type Serializable interface {
	Bytes() ([]byte, error)
}

type State interface {
	SetFromBytes(raw []byte) error
}

// LinearState models a state with a unique identifier that does not change through the evolution
// of the state.
type LinearState interface {
	// SetLinearID assigns the passed id to the state
	SetLinearID(id string) string
}

// Ownable models an ownable state
type Ownable interface {
	// Owners returns the identities of the owners of this state
	Owners() Identities
}

type AutoLinearState interface {
	GetLinearID() (string, error)
}

type EmbeddingState interface {
	GetState() interface{}
}

type Bytes struct {
	Payload []byte
}
