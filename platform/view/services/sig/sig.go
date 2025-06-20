/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sig

import (
	"context"

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

// Service models a repository of sign and verify keys.
type Service struct {
	sigService    driver.SigService
	sigRegistry   driver.SigRegistry
	auditRegistry driver.AuditRegistry
}

func NewService(sigService driver.SigService, sigRegistry driver.SigRegistry, auditRegistry driver.AuditRegistry) *Service {
	return &Service{
		sigService:    sigService,
		sigRegistry:   sigRegistry,
		auditRegistry: auditRegistry,
	}
}

// RegisterAuditInfo binds the passed audit info to the passed identity
func (s *Service) RegisterAuditInfo(ctx context.Context, identity view.Identity, info []byte) error {
	return s.auditRegistry.RegisterAuditInfo(ctx, identity, info)
}

// GetAuditInfo returns the audit info associated to the passed identity, nil if not found
func (s *Service) GetAuditInfo(ctx context.Context, identity view.Identity) ([]byte, error) {
	return s.auditRegistry.GetAuditInfo(ctx, identity)
}

// RegisterSigner binds the passed identity to the passed signer and verifier
func (s *Service) RegisterSigner(ctx context.Context, identity view.Identity, signer Signer, verifier Verifier) error {
	return s.sigRegistry.RegisterSigner(ctx, identity, signer, verifier)
}

// IsMe returns true if a signer was ever registered for the passed identity
func (s *Service) IsMe(ctx context.Context, identity view.Identity) bool {
	return s.sigService.IsMe(ctx, identity)
}

// AreMe returns the hashes of the passed identities that have a signer registered before
func (s *Service) AreMe(ctx context.Context, identities ...view.Identity) []string {
	return s.sigService.AreMe(ctx, identities...)
}

// RegisterVerifier binds the passed identity to the passed verifier
func (s *Service) RegisterVerifier(identity view.Identity, verifier Verifier) error {
	return s.sigRegistry.RegisterVerifier(identity, verifier)
}

// GetSigner returns the signer bound to the passed identity
func (s *Service) GetSigner(identity view.Identity) (Signer, error) {
	return s.sigService.GetSigner(identity)
}

// GetVerifier returns the verifier bound to the passed identity
func (s *Service) GetVerifier(identity view.Identity) (Verifier, error) {
	return s.sigService.GetVerifier(identity)
}

// GetSigningIdentity returns the signer identity bound to the passed identity
func (s *Service) GetSigningIdentity(identity view.Identity) (*SigningIdentity, error) {
	si, err := s.sigService.GetSigningIdentity(identity)
	if err != nil {
		return nil, err
	}
	return &SigningIdentity{si: si}, nil
}
