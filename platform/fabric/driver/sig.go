/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// VerifierProvider returns a Verifier for the passed identity
type VerifierProvider interface {
	// GetVerifier returns a Verifier for the passed identity
	GetVerifier(identity view.Identity) (Verifier, error)
}

type SigningIdentity interface {
	Serialize() ([]byte, error)
	Sign(msg []byte) ([]byte, error)
}

// Verifier is an interface which wraps the Verify method.
type Verifier = driver.Verifier

type Signer = driver.Signer

// SignerService models a signer service
type SignerService = driver.SigService
