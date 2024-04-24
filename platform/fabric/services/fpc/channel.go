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
	icp := &contractProvider{
		fns: p.FabricNetworkService,
		ch:  p.Channel,
	}

	return NewChaincode(
		p.Channel,
		p.ER,
		fpc.GetContract(icp, cid),
		&endorserContractImpl{fns: p.FabricNetworkService, ch: p.Channel, cid: cid},
		p.FabricNetworkService.IdentityProvider().DefaultIdentity(),
		p.FabricNetworkService.IdentityProvider(),
		cid,
	)
}

// GetDefaultChannel returns the default channel on which to invoke an FPC
func GetDefaultChannel(sp view.ServiceProvider) (*Channel, error) {
	fns, err := fabric.GetDefaultFNS(sp)
	if err != nil {
		return nil, err
	}
	ch, err := fns.Channel("")
	if err != nil {
		return nil, err
	}
	return newChannel(fns, ch, NewEnclaveRegistry(fns, ch)), nil
}

// GetChannel returns the channel for the passed network and channel name on which to invoke an FPC
func GetChannel(sp view.ServiceProvider, network, channelName string) (*Channel, error) {
	fns, err := fabric.GetFabricNetworkService(sp, network)
	if err != nil {
		return nil, err
	}
	ch, err := fns.Channel(channelName)
	if err != nil {
		return nil, err
	}
	return newChannel(fns, ch, NewEnclaveRegistry(fns, ch)), nil
}
