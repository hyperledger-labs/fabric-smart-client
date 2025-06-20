/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

//go:generate counterfeiter -o mock/id_provider.go -fake-name IdentityProvider . IdentityProvider

// IdentityProvider models the identity provider
type IdentityProvider interface {
	// DefaultIdentity returns the default identity known by this provider
	DefaultIdentity() view.Identity
	// Identity returns the identity bound to the passed label
	Identity(label string) view.Identity
	// Admins returns the identities of the administrators
	Admins() []view.Identity
	// Clients returns the identities of the clients of this node
	Clients() []view.Identity
}
