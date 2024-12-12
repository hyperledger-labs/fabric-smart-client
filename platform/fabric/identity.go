/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type IdentityProvider struct {
	localMembership driver.LocalMembership
	ip              driver.IdentityProvider
}

func (i *IdentityProvider) DefaultIdentity() view.Identity {
	return i.localMembership.DefaultIdentity()
}

func (i *IdentityProvider) Identity(label string) (view.Identity, error) {
	return i.ip.Identity(label)
}
