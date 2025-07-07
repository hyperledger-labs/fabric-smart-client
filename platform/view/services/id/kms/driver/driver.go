/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ConfigProvider interface {
	GetPath(s string) string
	GetStringSlice(key string) []string
	TranslatePath(path string) string
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

type KMSDriverName string

type NamedDriver struct {
	Name   KMSDriverName
	Driver Driver
}

// Driver models the key management interface
type Driver interface {
	// Load returns the signer, verifier and signing identity bound to the byte representation of passed pem encoded public key.
	Load(configProvider ConfigProvider) (view.Identity, Signer, Verifier, error)
}
