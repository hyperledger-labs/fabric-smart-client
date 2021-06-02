/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package id

import (
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type provider struct {
	defaultID view.Identity
	sp        view2.ServiceProvider
}

func NewProvider(sp view2.ServiceProvider) (*provider, error) {
	configProvider := view2.GetConfigService(sp)
	defaultID, err := Serialize(configProvider.GetPath("generic.identity.cert.file"))
	if err != nil {
		return nil, err
	}
	return &provider{sp: sp, defaultID: defaultID}, nil
}

func (p *provider) DefaultIdentity() view.Identity {
	return p.defaultID
}

func (p *provider) Identity(label string) view.Identity {
	id, err := view2.GetEndpointService(p.sp).GetIdentity(label, nil)
	if err != nil {
		panic(err)
	}
	return id
}
