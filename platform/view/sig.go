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

// Serialize returns the byte representation of this identity
func (i *Identity) Serialize() ([]byte, error) {
	return i.i.Serialize()
}

// Verify verifies the signature over the passed message.
func (i *Identity) Verify(message []byte, signature []byte) error {
	return i.i.Verify(message, signature)
}

// SigningIdentity models an identity that can sign, verify messages and whose public side
// can be serialized
type SigningIdentity struct {
	si driver.SigningIdentity
}

// Serialize returns the byte representation of the public side of this identity
func (s *SigningIdentity) Serialize() ([]byte, error) {
	return s.si.Serialize()
}

// Verify verifies the signature over the passed message.
func (s *SigningIdentity) Verify(message []byte, signature []byte) error {
	return s.si.Verify(message, signature)
}

// Sign signs message bytes and returns the signature or an error on failure.
func (s *SigningIdentity) Sign(message []byte) ([]byte, error) {
	return s.si.Sign(message)
}

func (s *SigningIdentity) GetPublicVersion() *Identity {
	return &Identity{i: s.si.GetPublicVersion()}
}

// SigService models a repository of sign and verify keys.
type SigService struct {
	sigService    driver.SigService
	sigRegistry   driver.SigRegistry
	auditRegistry driver.AuditRegistry
}

// RegisterAuditInfo binds the passed audit info to the passed identity
func (s *SigService) RegisterAuditInfo(identity view.Identity, info []byte) error {
	return s.auditRegistry.RegisterAuditInfo(identity, info)
}

// GetAuditInfo returns the audit info associated to the passed identity, nil if not found
func (s *SigService) GetAuditInfo(identity view.Identity) ([]byte, error) {
	return s.auditRegistry.GetAuditInfo(identity)
}

// RegisterSigner binds the passed identity to the passed signer and verifier
func (s *SigService) RegisterSigner(identity view.Identity, signer Signer, verifier Verifier) error {
	return s.sigRegistry.RegisterSigner(identity, signer, verifier)
}

// IsMe returns true if a signer was ever registered for the passed identity
func (s *SigService) IsMe(identity view.Identity) bool {
	return s.sigService.IsMe(identity)
}

// RegisterVerifier binds the passed identity to the passed verifier
func (s *SigService) RegisterVerifier(identity view.Identity, verifier Verifier) error {
	return s.sigRegistry.RegisterVerifier(identity, verifier)
}

// GetSigner returns the signer bound to the passed identity
func (s *SigService) GetSigner(identity view.Identity) (Signer, error) {
	return s.sigService.GetSigner(identity)
}

// GetVerifier returns the verifier bound to the passed identity
func (s *SigService) GetVerifier(identity view.Identity) (Verifier, error) {
	return s.sigService.GetVerifier(identity)
}

// GetSigningIdentity returns the signer identity bound to the passed identity
func (s *SigService) GetSigningIdentity(identity view.Identity) (*SigningIdentity, error) {
	si, err := s.sigService.GetSigningIdentity(identity)
	if err != nil {
		return nil, err
	}
	return &SigningIdentity{si: si}, nil
}

// GetSigService returns an instance of the sig service.
// It panics, if no instance is found.
func GetSigService(sp ServiceProvider) *SigService {
	return &SigService{
		sigService:    driver.GetSigService(sp),
		sigRegistry:   driver.GetSigRegistry(sp),
		auditRegistry: driver.GetAuditRegistry(sp),
	}
}
