/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// IdentityProvider models the identity provider
type IdentityProvider interface {
	// Identity returns the Fabric identity bound to the passed label.
	// If not Fabric identity is associated to the label, it returns the the SFC identity bound to that label.
	Identity(label string) view.Identity
}
