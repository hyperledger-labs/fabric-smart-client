/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// Chaincode returns a chaincode handler for the passed chaincode name
func (c *Channel) Chaincode(name string) driver.Chaincode {
	c.ChaincodesLock.RLock()
	ch, ok := c.Chaincodes[name]
	if ok {
		c.ChaincodesLock.RUnlock()
		return ch
	}
	c.ChaincodesLock.RUnlock()

	c.ChaincodesLock.Lock()
	defer c.ChaincodesLock.Unlock()
	ch, ok = c.Chaincodes[name]
	if ok {
		return ch
	}
	ch = chaincode.NewChaincode(
		name,
		c.NetworkConfig,
		c.ChannelConfig,
		c.Network.LocalMembership(),
		&PeerManager{Network: c.Network, Channel: c},
		c.Network.SignerService(),
		c.Network.OrderingService(),
		c,
		c,
	)
	c.Chaincodes[name] = ch
	return ch
}

type PeerManager struct {
	Network *Network
	Channel *Channel
}

func (p PeerManager) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.Client, error) {
	return p.Channel.NewPeerClientForAddress(cc)
}

func (p PeerManager) PickPeer(funcType driver.PeerFunctionType) *grpc.ConnectionConfig {
	return p.Network.PickPeer(funcType)
}
