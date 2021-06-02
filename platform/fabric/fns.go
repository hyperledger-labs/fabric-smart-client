/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type NetworkService struct {
	sp  view2.ServiceProvider
	fns api.FabricNetworkService
}

func (n *NetworkService) DefaultChannel() string {
	return n.fns.DefaultChannel()
}

func (n *NetworkService) Channels() []string {
	return n.fns.Channels()
}

func (n *NetworkService) GetTLSRootCert(party view.Identity) ([][]byte, error) {
	return n.fns.GetTLSRootCert(party)
}

// Peers returns the list of known Peer nodes
func (n *NetworkService) Peers() []*grpc.ConnectionConfig {
	return n.fns.Peers()
}

// Channel returns the channel service for the passed id
func (n *NetworkService) Channel(id string) (*Channel, error) {
	ch, err := n.fns.Channel(id)
	if err != nil {
		return nil, err
	}
	return &Channel{sp: n.sp, ch: ch}, nil
}

func (n *NetworkService) IdentityProvider() *IdentityProvider {
	return &IdentityProvider{
		localMembership: n.fns.LocalMembership(),
		ip:              n.fns.IdentityProvider(),
	}
}

func (n *NetworkService) LocalMembership() *LocalMembership {
	return &LocalMembership{
		network: n.fns,
	}
}

func (n *NetworkService) Ordering() *Ordering {
	return &Ordering{network: n.fns}
}

func (n *NetworkService) Name() string {
	// TODO: fix
	return "default"
}

func (n *NetworkService) ProcessorManager() *ProcessorManager {
	return &ProcessorManager{pm: n.fns.ProcessorManager()}
}

func (n *NetworkService) TransactionManager() *TransactionManager {
	return &TransactionManager{fns: n}
}

func (n *NetworkService) SigService() *SigService {
	return &SigService{sigService: n.fns.SigService()}
}

func GetFabricNetworkService(sp view2.ServiceProvider, id string) *NetworkService {
	fns, err := core.GetFabricNetworkServiceProvider(sp).FabricNetworkService(id)
	if err != nil {
		panic(err)
	}
	return &NetworkService{sp: sp, fns: fns}
}

func GetDefaultNetwork(sp view2.ServiceProvider) *NetworkService {
	return GetFabricNetworkService(sp, "")
}

// GetDefaultChannel returns the default channel of the default fns
func GetDefaultChannel(sp view2.ServiceProvider) *Channel {
	return GetChannelDefaultNetwork(sp, "")
}

// GetChannelDefaultNetwork returns the channel of the the default fns with the passed if
func GetChannelDefaultNetwork(sp view2.ServiceProvider, id string) *Channel {
	network := GetDefaultNetwork(sp)
	channel, err := network.Channel(id)
	if err != nil {
		panic(err)
	}

	return channel
}

func GetVaultDefaultNetwork(sp view2.ServiceProvider, id string) *Vault {
	network := GetDefaultNetwork(sp)
	channel, err := network.Channel(id)
	if err != nil {
		panic(err)
	}

	return channel.Vault()
}

func GetChannel(sp view2.ServiceProvider, networkID, channelID string) *Channel {
	network := GetFabricNetworkService(sp, networkID)
	channel, err := network.Channel(channelID)
	if err != nil {
		panic(err)
	}

	return channel
}

func GetVault(sp view2.ServiceProvider, networkID, channelID string) *Vault {
	network := GetFabricNetworkService(sp, networkID)
	channel, err := network.Channel(channelID)
	if err != nil {
		panic(err)
	}

	return channel.Vault()
}

func GetIdentityProvider(sp view2.ServiceProvider) *IdentityProvider {
	return GetDefaultNetwork(sp).IdentityProvider()
}

func GetLocalMembership(sp view2.ServiceProvider) *LocalMembership {
	return GetDefaultNetwork(sp).LocalMembership()
}
