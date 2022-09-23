/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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
	return &Vault{ch: c.ch}
}

func (c *Channel) Ledger() *Ledger {
	return &Ledger{ch: c}
}

func (c *Channel) MSPManager() *MSPManager {
	return &MSPManager{ch: c.ch}
}

func (c *Channel) Committer() *Committer {
	return c.committer
}

func (c *Channel) Finality() *Finality {
	return &Finality{ch: c.ch}
}

func (c *Channel) Chaincode(name string) *Chaincode {
	return &Chaincode{
		fns:           c.fns,
		chaincode:     c.ch.Chaincode(name),
		EventListener: newEventListener(c.sp, name),
	}
}

func (c *Channel) Delivery() *Delivery {
	return &Delivery{ch: c}
}

func (c *Channel) GetTLSRootCert(party view.Identity) ([][]byte, error) {
	return c.ch.GetTLSRootCert(party)
}

func (c *Channel) MetadataService() *MetadataService {
	return &MetadataService{ms: c.ch.MetadataService()}
}
