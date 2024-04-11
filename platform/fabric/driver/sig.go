/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type SigningIdentity interface {
	Serialize() ([]byte, error)
	Sign(msg []byte) ([]byte, error)
}

// Verifier is an interface which wraps the Verify method.
type Verifier interface {
	// Verify verifies the signature over the passed message.
	Verify(message, sigma []byte) error
}

type Signer interface {
	Sign(message []byte) ([]byte, error)
}

// SignerService models a signer service
type SignerService interface {
	// GetSigner returns the signer for the passed identity
	GetSigner(id view.Identity) (Signer, error)
	// GetSigningIdentity returns the signing identity for the passed identity
	GetSigningIdentity(id view.Identity) (SigningIdentity, error)
	// RegisterSigner register signer and verifier for the passed identity
	RegisterSigner(identity view.Identity, signer Signer, verifier Verifier) error
}
