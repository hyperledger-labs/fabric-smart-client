/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

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

type Identity struct {
	i driver.Identity
}

func (i *Identity) Serialize() ([]byte, error) {
	return i.i.Serialize()
}

func (i *Identity) Verify(message []byte, signature []byte) error {
	return i.i.Verify(message, signature)
}

type SigningIdentity struct {
	si driver.SigningIdentity
}

func (s *SigningIdentity) Serialize() ([]byte, error) {
	return s.si.Serialize()
}

func (s *SigningIdentity) Verify(message []byte, signature []byte) error {
	return s.si.Verify(message, signature)
}

func (s *SigningIdentity) Sign(raw []byte) ([]byte, error) {
	return s.si.Sign(raw)
}

func (s *SigningIdentity) GetPublicVersion() *Identity {
	return &Identity{i: s.si.GetPublicVersion()}
}

type SigService struct {
	sigService    driver.SigService
	sigRegistry   driver.SigRegistry
	auditRegistry driver.AuditRegistry
}

func (s *SigService) RegisterAuditInfo(identity view.Identity, info []byte) error {
	return s.auditRegistry.RegisterAuditInfo(identity, info)
}

func (s *SigService) GetAuditInfo(identity view.Identity) ([]byte, error) {
	return s.auditRegistry.GetAuditInfo(identity)
}

func (s *SigService) RegisterSigner(identity view.Identity, signer Signer, verifier Verifier) error {
	return s.sigRegistry.RegisterSigner(identity, signer, verifier)
}

func (s *SigService) RegisterVerifier(identity view.Identity, verifier Verifier) error {
	return s.sigRegistry.RegisterVerifier(identity, verifier)
}

func (s *SigService) GetSigner(identity view.Identity) (Signer, error) {
	return s.sigService.GetSigner(identity)
}

func (s *SigService) GetVerifier(identity view.Identity) (Verifier, error) {
	return s.sigService.GetVerifier(identity)
}

func (s *SigService) GetSigningIdentity(identity view.Identity) (*SigningIdentity, error) {
	si, err := s.sigService.GetSigningIdentity(identity)
	if err != nil {
		return nil, err
	}
	return &SigningIdentity{si: si}, nil
}

func GetSigService(sp ServiceProvider) *SigService {
	return &SigService{
		sigService:    driver.GetSigService(sp),
		sigRegistry:   driver.GetSigRegistry(sp),
		auditRegistry: driver.GetAuditRegistry(sp),
	}
}
