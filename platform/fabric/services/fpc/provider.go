/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/fpc/core/generic/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("fabric-sdk.fpc")

type provider struct {
	fns *fabric.NetworkService
	ch  *fabric.Channel

	er *EnclaveRegistry
}

func (p *provider) EnclaveRegistry() *EnclaveRegistry {
	return p.er
}

func (p *provider) Chaincode(cid string) *Chaincode {
	ep := &crypto.EncryptionProviderImpl{
		CSP: crypto.GetDefaultCSP(),
		GetCcEncryptionKey: func() ([]byte, error) {
			// Note that this function is called during EncryptionProvider.NewEncryptionContext()
			return p.er.ChaincodeEncryptionKey(cid)
		},
	}
	return &Chaincode{
		ep:  ep,
		ch:  p.ch,
		er:  p.er,
		id:  p.fns.IdentityProvider().DefaultIdentity(),
		ip:  p.fns.IdentityProvider(),
		cid: cid,
	}
}

func GetProvider(sp view.ServiceProvider) *provider {
	fns := fabric.GetDefaultFNS(sp)
	ch := fabric.GetDefaultChannel(sp)

	return &provider{
		fns: fns,
		ch:  ch,
		er: &EnclaveRegistry{
			fns: fns,
			ch:  ch,
		},
	}
}
