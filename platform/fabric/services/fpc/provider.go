/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"encoding/json"

	"github.com/pkg/errors"

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

	if resBoxed == nil || len(resBoxed.([]byte)) == 0 {
		return nil, nil
	}

	var res []string
	if err := json.Unmarshal(resBoxed.([]byte), &res); err != nil {
		return nil, errors.Wrapf(err, "failed unmarshalling [%s:%v]", string(resBoxed.([]byte)), resBoxed)
	}

	return res, nil
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
