/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/pkg/errors"
)

type CommitterService interface {
	driver.Finality
	driver.Committer
	ReloadConfigTransactions() error
	Commit(ctx context.Context, block *common.Block) error
}

type DeliveryService interface {
	driver.Delivery
	StoppableService
}

type StoppableService interface {
	Stop()
}

type Channel struct {
	ChannelName              string
	FinalityService          driver.Finality
	VaultService             driver.Vault
	VaultStoreService        driver.VaultStore
	ES                       driver.EnvelopeService
	TS                       driver.EndorserTransactionService
	MS                       driver.MetadataService
	DeliveryService          DeliveryService
	RWSetLoaderService       driver.RWSetLoader
	LedgerService            driver.Ledger
	ChannelMembershipService driver.ChannelMembership
	ChaincodeManagerService  driver.ChaincodeManager
	CommitterService         CommitterService
}

func (c *Channel) Init() error {
	if err := c.CommitterService.ReloadConfigTransactions(); err != nil {
		return errors.WithMessagef(err, "failed reloading config transactions")
	}
	return nil
}

func (c *Channel) Name() string {
	return c.ChannelName
}

func (c *Channel) Close() error {
	c.DeliveryService.Stop()
	return c.Vault().Close()
}

func (c *Channel) Vault() driver.Vault {
	return c.VaultService
}

func (c *Channel) VaultStore() driver.VaultStore {
	return c.VaultStoreService
}

func (c *Channel) Finality() driver.Finality {
	return c.FinalityService
}

func (c *Channel) Ledger() driver.Ledger {
	return c.LedgerService
}

func (c *Channel) Delivery() driver.Delivery {
	return c.DeliveryService
}

func (c *Channel) ChaincodeManager() driver.ChaincodeManager {
	return c.ChaincodeManagerService
}

func (c *Channel) ChannelMembership() driver.ChannelMembership {
	return c.ChannelMembershipService
}

func (c *Channel) RWSetLoader() driver.RWSetLoader {
	return c.RWSetLoaderService
}

func (c *Channel) Committer() driver.Committer {
	return c.CommitterService
}

func (c *Channel) EnvelopeService() driver.EnvelopeService {
	return c.ES
}

func (c *Channel) TransactionService() driver.EndorserTransactionService {
	return c.TS
}

func (c *Channel) MetadataService() driver.MetadataService {
	return c.MS
}
