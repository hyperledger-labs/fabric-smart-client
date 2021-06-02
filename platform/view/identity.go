/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// IdentityProvider provides identity services
type IdentityProvider struct {
	ip api.IdentityProvider
}

// DefaultIdentity returns the default identity
func (i *IdentityProvider) DefaultIdentity() view.Identity {
	return i.ip.DefaultIdentity()
}

// GetIdentityProvider returns an instance of the identity provider
func GetIdentityProvider(sp ServiceProvider) *IdentityProvider {
	return &IdentityProvider{ip: api.GetIdentityProvider(sp)}
}
