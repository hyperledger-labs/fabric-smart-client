/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package api

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// IdentityProvider models the identity provider
type IdentityProvider interface {
	// DefaultIdentity returns the default identity known by this provider
	DefaultIdentity() view.Identity

	// Identity returns the identity bound to the passed label
	Identity(label string) view.Identity
}
