/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fpc

import (
	fpc "github.com/hyperledger/fabric-private-chaincode/client_sdk/go/pkg/core/contract"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("fabric-sdk.fpc")

type channel struct {
	fns *fabric.NetworkService
	ch  *fabric.Channel

	er *EnclaveRegistry
}

func (p *channel) EnclaveRegistry() *EnclaveRegistry {
	return p.er
}

func (p *channel) Chaincode(cid string) *Chaincode {
	contractProvider := &contractProvider{
		fns: p.fns,
		ch:  p.ch,
	}
	return &Chaincode{
		ch:       p.ch,
		er:       p.er,
		contract: fpc.GetContract(contractProvider, cid),
		id:       p.fns.IdentityProvider().DefaultIdentity(),
		ip:       p.fns.IdentityProvider(),
		cid:      cid,
	}
}

func GetDefaultChannel(sp view.ServiceProvider) *channel {
	fns := fabric.GetDefaultFNS(sp)
	ch := fabric.GetDefaultChannel(sp)
	return &channel{
		fns: fns,
		ch:  ch,
		er: &EnclaveRegistry{
			fns: fns,
			ch:  ch,
		},
	}
}

func GetChannel(sp view.ServiceProvider, network, channelName string) *channel {
	fns := fabric.GetFabricNetworkService(sp, network)
	ch := fabric.GetChannel(sp, network, channelName)
	return &channel{
		fns: fns,
		ch:  ch,
		er: &EnclaveRegistry{
			fns: fns,
			ch:  ch,
		},
	}
}
