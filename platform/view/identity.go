/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// IdentityProvider provides identity services
type IdentityProvider struct {
	ip driver.IdentityProvider
}

// DefaultIdentity returns the default identity
func (i *IdentityProvider) DefaultIdentity() view.Identity {
	return i.ip.DefaultIdentity()
}

// Admins returns the identities of the administrators
func (i *IdentityProvider) Admins() []view.Identity {
	return i.ip.Admins()
}

// Clients returns the identities of the clients that can invoke views on this node
func (i *IdentityProvider) Clients() []view.Identity {
	return i.ip.Clients()
}

// Identity returns the identity bound to the passed label
func (i *IdentityProvider) Identity(label string) view.Identity {
	return i.ip.Identity(label)
}

// GetIdentityProvider returns an instance of the identity provider.
// It panics, if no instance is found.
func GetIdentityProvider(sp ServiceProvider) *IdentityProvider {
	return &IdentityProvider{ip: driver.GetIdentityProvider(sp)}
}
