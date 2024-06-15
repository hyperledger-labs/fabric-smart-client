/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type endpointService interface {
	GetIdentity(label string, pkiID []byte) (view.Identity, error)
}

type resolverService interface {
	// GetIdentity returns the identity associated to the passed label
	GetIdentity(label string) view.Identity
}

type resolver struct {
	rs resolverService
	es endpointService
}

func NewResolver(rs resolverService, es endpointService) (*resolver, error) {
	return &resolver{
		rs: rs,
		es: es,
	}, nil
}

func (e *resolver) GetIdentity(label string) (view.Identity, error) {
	res := e.rs.GetIdentity(label)
	if res.IsNone() {
		// lookup for a level up
		return e.es.GetIdentity(label, nil)

	}
	return res, nil
}
