/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/hyperledger/fabric-protos-go/common"
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

func (c *channel) ReloadConfigTransactions() error {
	c.applyLock.Lock()
	defer c.applyLock.Unlock()

	qe, err := c.vault.NewQueryExecutor()
	if err != nil {
		return errors.WithMessagef(err, "failed getting query executor")
	}
	defer qe.Done()

	logger.Infof("looking up the latest config block available")
	var sequence uint64 = 1
	for {
		txid := committer.ConfigTXPrefix + strconv.FormatUint(sequence, 10)
		vc, err := c.vault.Status(txid)
		if err != nil {
			panic(fmt.Sprintf("failed getting tx's status [%s], with err [%s]", txid, err))
		}
		done := false
		switch vc {
		case driver.Valid:
			txid := committer.ConfigTXPrefix + strconv.FormatUint(sequence, 10)
			logger.Infof("config block available, txid [%s], loading...", txid)

			key, err := rwset.CreateCompositeKey(channelConfigKey, []string{strconv.FormatUint(sequence, 10)})
			if err != nil {
				return errors.Wrapf(err, "cannot create configtx rws key")
			}
			envelope, err := qe.GetState(peerNamespace, key)
			if err != nil {
				return errors.Wrapf(err, "failed setting configtx state in rws")
			}
			env, err := protoutil.UnmarshalEnvelope(envelope)
			if err != nil {
				logger.Panicf("cannot get payload from config transaction [%s]: [%s]", txid, err)
			}
			payload, err := protoutil.UnmarshalPayload(env.Payload)
			if err != nil {
				logger.Panicf("cannot get payload from config transaction [%s]: [%s]", txid, err)
			}
			ctx, err := configtx.UnmarshalConfigEnvelope(payload.Data)
			if err != nil {
				err = errors.WithMessage(err, "error unmarshalling config which passed initial validity checks")
				logger.Criticalf("%+v", err)
				return err
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

			c.lock.Lock()
			c.resources = bundle
			c.lock.Unlock()

			sequence = sequence + 1
			continue
		case driver.Unknown:
			done = true
		default:
			panic(fmt.Sprintf("invalid configtx's [%s] status [%d]", txid, vc))
		}
		if done {
			break
		}
	}
	if sequence == 1 {
		logger.Infof("no config block available, must start from genesis")
		// no configuration block found
		return nil
	}
	logger.Infof("latest config block available at sequence [%d]", sequence-1)

	return nil
}

// CommitConfig is used to validate and apply configuration transactions for a channel.
func (c *channel) CommitConfig(blockNumber uint64, raw []byte, env *common.Envelope) error {
	commitConfigMutex.Lock()
	defer commitConfigMutex.Unlock()

	c.applyLock.Lock()
	defer c.applyLock.Unlock()

	logger.Debugf("[channel: %s] received config transaction number %d", c.name, blockNumber)

	if env == nil {
		return errors.Errorf("channel config found nil")
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

	if err := c.commitConfig(txid, blockNumber, ctx.Config.Sequence, raw); err != nil {
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
