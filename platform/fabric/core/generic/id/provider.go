/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type EndpointService interface {
	GetIdentity(label string) (view.Identity, error)
}

type provider struct {
	endpointService EndpointService
}

func NewProvider(endpointService EndpointService) (*provider, error) {
	return &provider{endpointService: endpointService}, nil
}

func (p *provider) Identity(label string) view.Identity {
	id, err := p.endpointService.GetIdentity(label)
	if err != nil {
		panic(err)
	}
	return id
}
