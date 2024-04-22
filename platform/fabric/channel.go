/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

type Channel struct {
	sp        view2.ServiceProvider
	fns       driver.FabricNetworkService
	ch        driver.Channel
	committer *Committer
}

func NewChannel(sp view2.ServiceProvider, fns driver.FabricNetworkService, ch driver.Channel) *Channel {
	return &Channel{sp: sp, fns: fns, ch: ch, committer: NewCommitter(ch)}
}

func (c *Channel) Name() string {
	return c.ch.Name()
}

func (c *Channel) Vault() *Vault {
	return newVault(c.ch)
}

func (c *Channel) Ledger() *Ledger {
	return &Ledger{l: c.ch.Ledger()}
}

func (c *Channel) MSPManager() *MSPManager {
	return &MSPManager{ch: c.ch.ChannelMembership()}
}

func (c *Channel) Committer() *Committer {
	return c.committer
}

func (c *Channel) Finality() *Finality {
	return &Finality{finality: c.ch.Finality()}
}

func (c *Channel) Chaincode(name string) *Chaincode {
	return &Chaincode{
		fns:           c.fns,
		chaincode:     c.ch.ChaincodeManager().Chaincode(name),
		EventListener: newEventListener(c.sp, name),
	}
}

func (c *Channel) Delivery() *Delivery {
	return &Delivery{delivery: c.ch.Delivery()}
}

func (c *Channel) MetadataService() *MetadataService {
	return &MetadataService{ms: c.ch.MetadataService()}
}

func (c *Channel) EnvelopeService() *EnvelopeService {
	return &EnvelopeService{ms: c.ch.EnvelopeService()}
}
