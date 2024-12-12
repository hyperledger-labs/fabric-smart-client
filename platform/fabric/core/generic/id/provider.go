/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
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

func (p *provider) Identity(label string) (view.Identity, error) {
	id, err := p.endpointService.GetIdentity(label)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting identity [%s]", label)
	}
	return id, nil
}
