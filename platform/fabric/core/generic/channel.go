/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"sync"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	xprotoutil "github.com/hyperledger/fabric-x-common/protoutil"

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
	// Platform is the driver name of the network the channel belongs to; it is
	// the platform of ChannelKey.
	Platform string

	// cancel tears down the context that bounds the lifetime of resources owned by
	// the channel's dependencies (e.g. the ChaincodeManagerService's per-chaincode
	// discovery caches). It is invoked in Close.
	//
	// There is no channel-scoped context flowing from above the
	// NewChannel/ChannelProvider construction path today, so that context is rooted
	// at context.Background() in the provider; if that ever changes, it should
	// derive from the higher-level context instead.
	cancel context.CancelFunc

	keyMu sync.Mutex
	key   *driver.ChannelKey
}

func (c *Channel) Init() error {
	if err := c.CommitterService.ReloadConfigTransactions(); err != nil {
		return errors.WithMessage(err, "failed reloading config transactions")
	}
	return nil
}

func (c *Channel) Name() string {
	return c.ChannelName
}

// ChannelKey returns a copy of the key of the channel, reading block 0 from
// LedgerService on the first successful call. A failed read is not cached, so a
// later call retries.
func (c *Channel) ChannelKey() (*driver.ChannelKey, error) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	if c.key != nil {
		k := *c.key
		return &k, nil
	}
	b, err := c.LedgerService.GetBlockByNumber(0)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed reading block 0 of channel [%s]", c.ChannelName)
	}
	cb, ok := b.(interface{ GetHeader() *common.BlockHeader })
	if !ok || cb.GetHeader() == nil {
		return nil, errors.Errorf("block 0 of channel [%s] has no header", c.ChannelName)
	}
	c.key = &driver.ChannelKey{
		Platform:    c.Platform,
		Channel:     c.ChannelName,
		GenesisHash: xprotoutil.BlockHeaderHash(cb.GetHeader()),
	}
	k := *c.key
	return &k, nil
}

func (c *Channel) Close() error {
	c.DeliveryService.Stop()
	// Cancelling is enough to stop the ChaincodeManagerService: its context, and in
	// turn every per-chaincode discovery cache, derives from the one cancel closes.
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
