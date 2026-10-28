/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	viewapi "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// IdentityProvider models the identity provider for the view service.
type IdentityProvider interface {
	// DefaultIdentity returns the node's default identity, which is always authorized.
	DefaultIdentity() viewapi.Identity
	// Clients returns the additional identities authorized to submit commands (fsc.view.remote.clients).
	Clients() []viewapi.Identity
}

// VerifierProvider models the verifier provider for the view service.
type VerifierProvider interface {
	GetVerifier(identity viewapi.Identity) (sig.Verifier, error)
}

// AccessControlChecker is responsible for performing policy based access control checks.
type AccessControlChecker struct {
	IdentityProvider IdentityProvider
	VerifierProvider VerifierProvider
}

// NewAccessControlChecker returns a new instance of the access control checker.
func NewAccessControlChecker(identityProvider IdentityProvider, verifierProvider VerifierProvider) *AccessControlChecker {
	return &AccessControlChecker{IdentityProvider: identityProvider, VerifierProvider: verifierProvider}
}

// Check checks if the given command is authorized: its creator must be the node's default
// identity or one of the client identities, and the command signature must verify under it.
func (a *AccessControlChecker) Check(sc *protos.SignedCommand, c *protos.Command) error {
	// Is the creator recognized
	validIdentities := append([]viewapi.Identity{a.IdentityProvider.DefaultIdentity()}, a.IdentityProvider.Clients()...)

	found := false
	for _, identity := range validIdentities {
		if identity.Equal(c.Header.Creator) {
			found = true
			break
		}
	}

	if !found {
		return errors.Wrapf(view.ErrIdentityNotRecognized, "identity [%s] not recognized", viewapi.Identity(c.Header.Creator))
	}

	verifier, err := a.VerifierProvider.GetVerifier(c.Header.Creator)
	if err != nil {
		return errors.WithMessagef(err, "failed getting verifier for [%s]", viewapi.Identity(c.Header.Creator))
	}

	if err := verifier.Verify(sc.Command, sc.Signature); err != nil {
		return errors.WithMessagef(err, "failed verifying signature from [%s]", viewapi.Identity(c.Header.Creator))
	}

	return nil
}
