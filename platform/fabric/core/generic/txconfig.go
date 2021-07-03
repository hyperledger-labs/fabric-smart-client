/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

const (
	channelConfigKey = "CHANNEL_CONFIG_ENV_BYTES"
	peerNamespace    = "_configtx"
)

// TODO: introduced due to a race condition in idemix.
var commitConfigMutex = &sync.Mutex{}

// CommitConfig is used to validate and apply configuration transactions for a channel.
func (c *channel) CommitConfig(blockNumber uint64, envelope []byte) error {
	commitConfigMutex.Lock()
	defer commitConfigMutex.Unlock()

	c.applyLock.Lock()
	defer c.applyLock.Unlock()

	if len(envelope) == 0 {
		return errors.Errorf("channel config found nil")
	}

	env, err := protoutil.UnmarshalEnvelope(envelope)
	if err != nil {
		logger.Panicf("Cannot get payload from config transaction [%s]: [%s]", blockNumber, err)
	}

	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		logger.Panicf("Cannot get payload from config transaction [%s]: [%s]", blockNumber, err)
	}

	ctx, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		err = errors.WithMessage(err, "error unmarshalling config which passed initial validity checks")
		logger.Criticalf("%+v", err)
		return err
	}

	txid := committer.ConfigTXPrefix + strconv.FormatUint(ctx.Config.Sequence, 10)
	vc, err := c.vault.Status(txid)
	if err != nil {
		panic(fmt.Sprintf("failed getting tx's status [%s], with err [%s]", txid, err))
	}
	switch vc {
	case driver.Valid:
		return nil
	case driver.Unknown:
		// this is okay
	default:
		panic(fmt.Sprintf("invalid configtx's [%s] status [%d]", txid, vc))
	}

	var bundle *channelconfig.Bundle
	if c.Resources() == nil {
		// setup the genesis block
		bundle, err = channelconfig.NewBundle(c.name, ctx.Config, factory.GetDefault())
		if err != nil {
			return err
		}
	} else {
		configTxValidator := c.Resources().ConfigtxValidator()
		err := configTxValidator.Validate(ctx)
		if err != nil {
			return err
		}

		bundle, err = channelconfig.NewBundle(configTxValidator.ChannelID(), ctx.Config, factory.GetDefault())
		if err != nil {
			return err
		}

		channelconfig.LogSanityChecks(bundle)
		capabilitiesSupportedOrPanic(bundle)
	}

	if err := c.commitConfig(txid, blockNumber, ctx.Config.Sequence, envelope); err != nil {
		return errors.Wrapf(err, "failed committing configtx to the vault")
	}

	c.lock.Lock()
	c.resources = bundle
	c.lock.Unlock()

	return nil
}

// Resources returns the active channel configuration bundle.
func (c *channel) Resources() channelconfig.Resources {
	c.lock.RLock()
	res := c.resources
	c.lock.RUnlock()
	return res
}

func (c *channel) commitConfig(txid string, blockNumber uint64, seq uint64, envelope []byte) error {
	rws, err := c.vault.NewRWSet(txid)
	if err != nil {
		return errors.Wrapf(err, "cannot create rws for configtx")
	}
	defer rws.Done()

	key, err := rwset.CreateCompositeKey(channelConfigKey, []string{strconv.FormatUint(seq, 10)})
	if err != nil {
		return errors.Wrapf(err, "cannot create configtx rws key")
	}
	if err := rws.SetState(peerNamespace, key, envelope); err != nil {
		return errors.Wrapf(err, "failed setting configtx state in rws")
	}
	rws.Done()
	if err := c.CommitTX(txid, blockNumber, 0, nil); err != nil {
		if err2 := c.DiscardTx(txid); err2 != nil {
			logger.Errorf("failed committing configtx rws [%s]", err2)
		}
		return errors.Wrapf(err, "failed committing configtx rws")
	}
	return nil
}

func capabilitiesSupportedOrPanic(res channelconfig.Resources) {
	ac, ok := res.ApplicationConfig()
	if !ok {
		logger.Panicf("[channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		logger.Panicf("[channel %s] incompatible: %s", res.ConfigtxValidator().ChannelID(), err)
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		logger.Panicf("[channel %s] incompatible: %s", res.ConfigtxValidator().ChannelID(), err)
	}
}
