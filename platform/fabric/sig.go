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
// A verifier it deserializes is cached for the life of the node; use
// LookupVerifier for identities chosen by a remote party.
func (s *SignerService) GetVerifier(id view.Identity) (Verifier, error) {
	return s.sigService.GetVerifier(id)
}

// verifierLookup is implemented by signature services that can resolve a
// verifier without caching it, such as sig.Service.
type verifierLookup interface {
	LookupVerifier(id view.Identity) (Verifier, error)
}

// LookupVerifier returns the verifier for id like GetVerifier, but does not
// cache a verifier it has to deserialize, so identities chosen by a remote
// party cannot grow the signature service's cache. A signature service that
// cannot look up without caching falls back to GetVerifier.
func (s *SignerService) LookupVerifier(id view.Identity) (Verifier, error) {
	if l, ok := s.sigService.(verifierLookup); ok {
		return l.LookupVerifier(id)
	}
	return s.sigService.GetVerifier(id)
}
