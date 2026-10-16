/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// VerifyingIdentity is an identity that can verify signatures produced by its signing counterpart.
type VerifyingIdentity interface {
	// Serialize returns the byte representation of this identity
	Serialize() ([]byte, error)

	// Verify verifies the signature over the passed message.
	Verify(message, signature []byte) error
}

// SigningIdentity is an identity that holds the private material to sign messages.
type SigningIdentity interface {
	VerifyingIdentity

	// Sign signs message bytes and returns the signature or an error on failure.
	Sign(message []byte) ([]byte, error)

	// GetPublicVersion returns the verifying counterpart of this identity.
	GetPublicVersion() VerifyingIdentity
}

// Signer is an interface which wraps the Sign method.
type Signer interface {
	// Sign signs message bytes and returns the signature or an error on failure.
	Sign(message []byte) ([]byte, error)
}

// Verifier is an interface which wraps the Verify method.
type Verifier interface {
	// Verify verifies the signature over the passed message.
	Verify(message, sigma []byte) error
}

// SigService models a repository of sign and verify keys.
type SigService interface {
	// GetSigner returns the signer bound to the passed identity
	GetSigner(identity view.Identity) (Signer, error)

	// GetVerifier returns the verifier bound to the passed identity
	GetVerifier(identity view.Identity) (Verifier, error)

	// GetSigningIdentity returns the signer identity bound to the passed identity
	GetSigningIdentity(identity view.Identity) (SigningIdentity, error)

	// IsMe returns true if a signer was ever registered for the passed identity
	IsMe(ctx context.Context, identity view.Identity) bool

	// AreMe returns the hashes of the passed identities that have a signer registered before
	AreMe(ctx context.Context, identities ...view.Identity) []string
}

// SigDeserializer turns serialized identities into signers and verifiers.
type SigDeserializer interface {
	// DeserializeVerifier returns the verifier for the passed serialized identity.
	DeserializeVerifier(raw []byte) (Verifier, error)

	// DeserializeSigner returns the signer for the passed serialized identity.
	DeserializeSigner(raw []byte) (Signer, error)

	// Info returns a human-readable description of the passed serialized identity.
	// auditInfo is optional: identity types that support it (e.g. idemix) use it to add
	// the enrollment ID and return an error if it does not match raw; others ignore it.
	Info(raw, auditInfo []byte) (string, error)
}
