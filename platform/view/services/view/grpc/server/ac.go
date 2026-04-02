/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// IdentityProvider models the identity provider for the view service.
type IdentityProvider interface {
	DefaultIdentity() view2.Identity
	Admins() []view2.Identity
	Clients() []view2.Identity
}

// VerifierProvider models the verifier provider for the view service.
type VerifierProvider interface {
	GetVerifier(identity view2.Identity) (sig.Verifier, error)
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

// Check checks if the given command is authorized.
func (a *AccessControlChecker) Check(sc *protos2.SignedCommand, c *protos2.Command) error {
	// Is the creator recognized
	validIdentities := []view2.Identity{a.IdentityProvider.DefaultIdentity()}
	admins := a.IdentityProvider.Admins()
	if len(admins) != 0 {
		validIdentities = append(validIdentities, admins...)
	}
	clients := a.IdentityProvider.Clients()
	if len(clients) != 0 {
		validIdentities = append(validIdentities, clients...)
	}

	found := false
	for _, identity := range validIdentities {
		if identity.Equal(c.Header.Creator) {
			found = true
			break
		}
	}

	if !found {
		return errors.Wrapf(view.ErrIdentityNotRecognized, "identity [%s] not recognized", view2.Identity(c.Header.Creator))
	}

	verifier, err := a.VerifierProvider.GetVerifier(c.Header.Creator)
	if err != nil {
		return errors.WithMessagef(err, "failed getting verifier for [%s]", view2.Identity(c.Header.Creator))
	}

	if err := verifier.Verify(sc.Command, sc.Signature); err != nil {
		return errors.WithMessagef(err, "failed verifying signature from [%s]", view2.Identity(c.Header.Creator))
	}

	return nil
}
