/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// SignerService models a signer service
type SignerService interface {
	// GetSigner returns the signer for the passed identity
	GetSigner(id view.Identity) (Signer, error)
	// GetSigningIdentity returns the signing identity for the passed identity
	GetSigningIdentity(id view.Identity) (SigningIdentity, error)
	// RegisterSigner register signer and verifier for the passed identity
	RegisterSigner(identity view.Identity, signer Signer, verifier Verifier) error
}
