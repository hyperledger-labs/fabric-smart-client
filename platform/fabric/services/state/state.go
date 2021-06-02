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

type LinearState interface {
	SetLinearID(id string) string
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
