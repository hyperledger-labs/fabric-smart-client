/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Identity interface {
	// Serialize returns the byte representation of this identity
	Serialize() ([]byte, error)

	// Verify verifies the signature over the passed message.
	Verify(message []byte, signature []byte) error
}

type SigningIdentity interface {
	Identity

	// Sign signs message bytes and returns the signature or an error on failure.
	Sign(message []byte) ([]byte, error)

	GetPublicVersion() Identity
}

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

//go:generate counterfeiter -o mock/sig_service.go -fake-name SigService . SigService

// SigService models a repository of sign and verify keys.
type SigService interface {
	// GetSigner returns the signer bound to the passed identity
	GetSigner(identity view.Identity) (Signer, error)

	// GetVerifier returns the verifier bound to the passed identity
	GetVerifier(identity view.Identity) (Verifier, error)

	// GetSigningIdentity returns the signer identity bound to the passed identity
	GetSigningIdentity(identity view.Identity) (SigningIdentity, error)

	// IsMe returns true if a signer was ever registered for the passed identity
	IsMe(identity view.Identity) bool
}

func GetSigService(sp ServiceProvider) SigService {
	s, err := sp.GetService(reflect.TypeOf((*SigService)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(SigService)
}

// AuditRegistry models a repository of identities' audit information
type AuditRegistry interface {
	// RegisterAuditInfo binds the passed audit info to the passed identity
	RegisterAuditInfo(identity view.Identity, info []byte) error

	// GetAuditInfo returns the audit info associated to the passed identity, nil if not found
	GetAuditInfo(identity view.Identity) ([]byte, error)
}

func GetAuditRegistry(sp ServiceProvider) AuditRegistry {
	s, err := sp.GetService(reflect.TypeOf((*AuditRegistry)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(AuditRegistry)
}

type SigRegistry interface {
	// RegisterSigner binds the passed identity to the passed signer and verifier
	RegisterSigner(identity view.Identity, signer Signer, verifier Verifier) error

	// RegisterVerifier binds the passed identity to the passed verifier
	RegisterVerifier(identity view.Identity, verifier Verifier) error
}

func GetSigRegistry(sp ServiceProvider) SigRegistry {
	s, err := sp.GetService(reflect.TypeOf((*SigRegistry)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(SigRegistry)
}
