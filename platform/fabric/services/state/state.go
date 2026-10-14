/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

// Serializable is implemented by states that provide their own serialization.
// JSONCodec.Marshal uses Bytes instead of JSON encoding for such states.
type Serializable interface {
	Bytes() ([]byte, error)
}

// State is implemented by states that provide their own deserialization.
// JSONCodec.Unmarshal uses SetFromBytes instead of JSON decoding for such states.
type State interface {
	SetFromBytes(raw []byte) error
}

// LinearState models a state with a unique identifier that does not change through the evolution
// of the state.
type LinearState interface {
	// SetLinearID receives a freshly generated id and returns the id the state is stored under,
	// which may differ from the one passed in.
	SetLinearID(id string) string
}

// Ownable models an ownable state
type Ownable interface {
	// Owners returns the identities of the owners of this state
	Owners() Identities
}

// AutoLinearState models a state that derives its own unique identifier.
// It takes precedence over LinearState when a state implements both.
type AutoLinearState interface {
	GetLinearID() (string, error)
}

// EmbeddingState models a wrapper around another state.
// The wrapper is stored under the identifier of the state returned by GetState.
type EmbeddingState interface {
	GetState() any
}
