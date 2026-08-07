/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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
	ES                       driver.EnvelopeService
	TS                       driver.EndorserTransactionService
	MS                       driver.MetadataService
	DeliveryService          DeliveryService
	RWSetLoaderService       driver.RWSetLoader
	LedgerService            driver.Ledger
	ChannelMembershipService driver.ChannelMembership
	ChaincodeManagerService  driver.ChaincodeManager
	CommitterService         CommitterService

	// ctx/cancel bound the lifetime of resources owned by the channel's
	// dependencies (e.g. the ChaincodeManagerService's per-chaincode
	// discovery caches). cancel is invoked in Close.
	//
	// There is no channel-scoped context flowing from above the
	// NewChannel/ChannelProvider construction path today, so ctx is rooted
	// at context.Background() here; if that ever changes, this should
	// derive from the higher-level context instead.
	ctx    context.Context
	cancel context.CancelFunc
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
	if stopper, ok := c.ChaincodeManagerService.(StoppableService); ok {
		stopper.Stop()
	}
	if c.cancel != nil {
		c.cancel()
	}
	return c.Vault().Close()
}

func (c *Channel) Vault() driver.Vault {
	return c.VaultService
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
