/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type enclaveRegistry struct {
	fns *fabric.NetworkService
	ch  *fabric.Channel
}

func (e *enclaveRegistry) ListProvisionedEnclaves(cid string) ([]string, error) {
	resBoxed, err := e.ch.Chaincode("ercc").Query(
		"QueryListProvisionedEnclaves", cid,
	).WithInvokerIdentity(
		e.fns.IdentityProvider().DefaultIdentity(),
	).WithEndorsers(
		e.fns.IdentityProvider().Identity("Org1_peer_0"),
		e.fns.IdentityProvider().Identity("Org2_peer_0"),
	).Call()
	if err != nil {
		return nil, err
	}
	return resBoxed.([]string), nil
}

type provider struct {
	er *enclaveRegistry
}

func (p *provider) EnclaveRegistry() *enclaveRegistry {
	return p.er
}

func GetProvider(sp view.ServiceProvider) *provider {
	fns := fabric.GetDefaultFNS(sp)
	ch := fabric.GetDefaultChannel(sp)

	return &provider{
		er: &enclaveRegistry{
			fns: fns,
			ch:  ch,
		},
	}
}
