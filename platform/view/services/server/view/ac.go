/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/pkg/errors"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type IdentityProvider interface {
	DefaultIdentity() view.Identity
	Admins() []view.Identity
}

type VerifierProvider interface {
	GetVerifier(identity view.Identity) (view2.Verifier, error)
}

type AccessControlChecker struct {
	IdentityProvider IdentityProvider
	VerifierProvider VerifierProvider
}

func NewAccessControlChecker(identityProvider IdentityProvider, verifierProvider VerifierProvider) *AccessControlChecker {
	return &AccessControlChecker{IdentityProvider: identityProvider, VerifierProvider: verifierProvider}
}

func (a *AccessControlChecker) Check(sc *protos2.SignedCommand, c *protos2.Command) error {
	// Is the creator recognized
	validIdentities := []view.Identity{a.IdentityProvider.DefaultIdentity()}
	admins := a.IdentityProvider.Admins()
	if len(admins) != 0 {
		validIdentities = append(validIdentities, admins...)
	}

	found := false
	for _, identity := range validIdentities {
		if identity.Equal(c.Header.Creator) {
			found = true
			break
		}
	}

	if !found {
		return errors.Errorf("identity [%s] not recognized", view.Identity(c.Header.Creator))
	}

	verifier, err := a.VerifierProvider.GetVerifier(c.Header.Creator)
	if err != nil {
		return errors.WithMessagef(err, "failed getting verifier for [%s]", view.Identity(c.Header.Creator))
	}

	if err := verifier.Verify(sc.Command, sc.Signature); err != nil {
		return errors.WithMessagef(err, "failed verifying signature from [%s]", view.Identity(c.Header.Creator))
	}

	return nil
}
