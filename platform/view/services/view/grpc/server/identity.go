/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

//go:generate counterfeiter -o client/mock/identity.go -fake-name Identity . Identity

// Identity refers to the creator of a transaction or command.
type Identity interface {
	// Serialize returns the serialized representation of the identity.
	Serialize() ([]byte, error)
}

// SigningIdentity defines the functions necessary to sign an array of bytes.
// It is needed to sign the commands.
type SigningIdentity interface {
	Identity // extends Identity

	// Sign signs the given message.
	Sign(msg []byte) ([]byte, error)
}
