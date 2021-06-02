/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/generic/api"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type IdentityProvider struct {
	ip api.IdentityProvider
}

func (i *IdentityProvider) DefaultIdentity() view.Identity {
	return i.ip.DefaultIdentity()
}

func (i *IdentityProvider) Identity(label string) view.Identity {
	return i.ip.Identity(label)
}

func GetIdentityProvider(sp view2.ServiceProvider) *IdentityProvider {
	return &IdentityProvider{ip: api.GetIdentityProvider(sp)}
}
