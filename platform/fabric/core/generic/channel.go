/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"bytes"
	"context"
	"sync"
	"time"

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

	keyMu      sync.Mutex
	key        *driver.ChannelKey
	keyErr     error
	keyRetryAt time.Time
	keyFetch   chan struct{}
	// keyTimeout overrides channelKeyTimeout when set.
	keyTimeout time.Duration
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

const (
	// channelKeyTimeout bounds how long ChannelKey waits for block 0. The ledger
	// API takes no context, so a read that outlives it keeps running and its
	// result is still cached when it arrives.
	channelKeyTimeout = 10 * time.Second
	// channelKeyRetry is how long ChannelKey reports a failed or timed-out read
	// before reading block 0 again, so repeated requests for an unreadable channel
	// do not each query the ledger.
	channelKeyRetry = 5 * time.Second
)

// ChannelKey returns a copy of the key of the channel, reading block 0 from
// LedgerService on first use. Concurrent callers share one read. A read that fails
// or takes longer than channelKeyTimeout makes ChannelKey fail; the failure is
// reported to callers for channelKeyRetry before block 0 is read again.
func (c *Channel) ChannelKey() (*driver.ChannelKey, error) {
	c.keyMu.Lock()
	switch {
	case c.key != nil:
		key := c.key
		c.keyMu.Unlock()
		return cloneKey(key), nil
	case c.keyErr != nil && time.Now().Before(c.keyRetryAt):
		err := c.keyErr
		c.keyMu.Unlock()
		return nil, err
	}
	if c.keyFetch == nil {
		done := make(chan struct{})
		c.keyFetch = done
		go c.fetchKey(done)
	}
	done := c.keyFetch
	timeout := c.keyTimeout
	c.keyMu.Unlock()
	if timeout == 0 {
		timeout = channelKeyTimeout
	}

	select {
	case <-done:
	case <-time.After(timeout):
		err := errors.Errorf("timed out reading block 0 of channel [%s]", c.ChannelName)
		c.keyMu.Lock()
		if c.key == nil {
			c.keyErr, c.keyRetryAt = err, time.Now().Add(channelKeyRetry)
		}
		c.keyMu.Unlock()
		return nil, err
	}
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	if c.key != nil {
		return cloneKey(c.key), nil
	}
	return nil, c.keyErr
}

// fetchKey reads block 0, records the key or the failure, and closes done.
func (c *Channel) fetchKey(done chan struct{}) {
	key, err := c.readKey()
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	if err != nil {
		c.keyErr, c.keyRetryAt = err, time.Now().Add(channelKeyRetry)
	} else {
		c.key, c.keyErr = key, nil
	}
	c.keyFetch = nil
	close(done)
}

func (c *Channel) readKey() (*driver.ChannelKey, error) {
	b, err := c.LedgerService.GetBlockByNumber(0)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed reading block 0 of channel [%s]", c.ChannelName)
	}
	cb, ok := b.(interface{ GetHeader() *common.BlockHeader })
	if !ok || cb.GetHeader() == nil {
		return nil, errors.Errorf("block 0 of channel [%s] has no header", c.ChannelName)
	}
	return &driver.ChannelKey{
		Platform:    c.Platform,
		Channel:     c.ChannelName,
		GenesisHash: xprotoutil.BlockHeaderHash(cb.GetHeader()),
	}, nil
}

func cloneKey(k *driver.ChannelKey) *driver.ChannelKey {
	c := *k
	c.GenesisHash = bytes.Clone(k.GenesisHash)
	return &c
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
