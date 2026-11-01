/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type SignerService struct {
	sigService driver.SignerService
}

func (s *SignerService) GetSigner(id view.Identity) (Signer, error) {
	return s.sigService.GetSigner(id)
}

// GetVerifier returns the verifier for id from the network's signature
// service, deserializing id with the deserializers of the node's locally
// configured MSPs if no verifier is registered for it. It is not a membership
// check. An x509 identity deserializes to a verifier for its public key
// without being validated against any MSP, so a verified signature shows that
// the signer holds the key for id, not that id belongs to an MSP. An idemix
// identity deserializes only if the node has a local idemix MSP from the same
// issuer, and a local idemix MSP loads only with the node's own credential.
func (s *SignerService) GetVerifier(id view.Identity) (Verifier, error) {
	return s.sigService.GetVerifier(id)
}
