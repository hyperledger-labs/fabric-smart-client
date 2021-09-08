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

// Channel models a Fabric channel that supports invocation of a Fabric Private Chaincode
type Channel struct {
	FabricNetworkService *fabric.NetworkService
	Channel              *fabric.Channel

	ER *EnclaveRegistry
}

func newChannel(fns *fabric.NetworkService, ch *fabric.Channel, er *EnclaveRegistry) *Channel {
	return &Channel{FabricNetworkService: fns, Channel: ch, ER: er}
}

// EnclaveRegistry returns the enclave registry for this channel
func (p *Channel) EnclaveRegistry() *EnclaveRegistry {
	return p.ER
}

// Chaincode returns a wrapper around the Fabric Private Chaincode whose name is cid
func (p *Channel) Chaincode(cid string) *Chaincode {
	contractProvider := &contractProvider{
		fns: p.FabricNetworkService,
		ch:  p.Channel,
	}
	return NewChaincode(
		p.Channel,
		p.ER,
		fpc.GetContract(contractProvider, cid),
		p.FabricNetworkService.IdentityProvider().DefaultIdentity(),
		p.FabricNetworkService.IdentityProvider(),
		cid,
	)
}

// GetDefaultChannel returns the default channel on which to invoke an FPC
func GetDefaultChannel(sp view.ServiceProvider) *Channel {
	fns := fabric.GetDefaultFNS(sp)
	ch := fabric.GetDefaultChannel(sp)
	return newChannel(fns, ch, NewEnclaveRegistry(fns, ch))
}

// GetChannel returns the channel for the passed network and channel name on which to invoke an FPC
func GetChannel(sp view.ServiceProvider, network, channelName string) *Channel {
	fns := fabric.GetFabricNetworkService(sp, network)
	if fns == nil {
		return nil
	}
	ch := fabric.GetChannel(sp, network, channelName)
	return newChannel(fns, ch, NewEnclaveRegistry(fns, ch))
}
