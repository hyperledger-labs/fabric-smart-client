/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"strconv"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

const (
	channelConfigKey = "CHANNEL_CONFIG_ENV_BYTES"
	peerNamespace    = "_configtx"
)

// TODO: introduced due to a race condition in idemix.
var commitConfigMutex = &sync.Mutex{}

func (c *Channel) ReloadConfigTransactions() error {
	c.ResourcesApplyLock.Lock()
	defer c.ResourcesApplyLock.Unlock()

	qe, err := c.Vault.NewQueryExecutor()
	if err != nil {
		return errors.WithMessagef(err, "failed getting query executor")
	}
	defer qe.Done()

	logger.Infof("looking up the latest config block available")
	var sequence uint64 = 0
	for {
		txID := committer.ConfigTXPrefix + strconv.FormatUint(sequence, 10)
		vc, _, err := c.Vault.Status(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed getting tx's status [%s]", txID)
		}
		logger.Infof("check config block at txID [%s], status [%v]...", txID, vc)
		done := false
		switch vc {
		case driver.Valid:
			logger.Infof("config block available, txID [%s], loading...", txID)

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
				return errors.Wrapf(err, "cannot get payload from config transaction [%s]", txID)
			}
			payload, err := protoutil.UnmarshalPayload(env.Payload)
			if err != nil {
				return errors.Wrapf(err, "cannot get payload from config transaction [%s]", txID)
			}
			ctx, err := configtx.UnmarshalConfigEnvelope(payload.Data)
			if err != nil {
				return errors.Wrapf(err, "error unmarshalling config which passed initial validity checks [%s]", txID)
			}

			var bundle *channelconfig.Bundle
			if c.Resources() == nil {
				// set up the genesis block
				bundle, err = channelconfig.NewBundle(c.ChannelName, ctx.Config, factory.GetDefault())
				if err != nil {
					return errors.Wrapf(err, "failed to build a new bundle")
				}
			} else {
				configTxValidator := c.Resources().ConfigtxValidator()
				err := configTxValidator.Validate(ctx)
				if err != nil {
					return errors.Wrapf(err, "failed to validate config transaction [%s]", txID)
				}

				bundle, err = channelconfig.NewBundle(configTxValidator.ChannelID(), ctx.Config, factory.GetDefault())
				if err != nil {
					return errors.Wrapf(err, "failed to create next bundle")
				}

				channelconfig.LogSanityChecks(bundle)
				if err := capabilitiesSupported(bundle); err != nil {
					return err
				}
			}

			if err := c.ApplyBundle(bundle); err != nil {
				return err
			}

			sequence = sequence + 1
			continue
		case driver.Unknown:
			if sequence == 0 {
				// Give a chance to 1, in certain setting the first block starts with 1
				sequence++
				continue
			}

			logger.Infof("config block at txID [%s] unavailable, stop loading", txID)
			done = true
		default:
			return errors.Errorf("invalid configtx's [%s] status [%d]", txID, vc)
		}
		if done {
			logger.Infof("loading config block done")
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

// CommitConfig is used to validate and apply configuration transactions for a Channel.
func (c *Channel) CommitConfig(blockNumber uint64, raw []byte, env *common.Envelope) error {
	commitConfigMutex.Lock()
	defer commitConfigMutex.Unlock()

	c.ResourcesApplyLock.Lock()
	defer c.ResourcesApplyLock.Unlock()

	if env == nil {
		return errors.Errorf("Channel config found nil")
	}

	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return errors.Wrapf(err, "cannot get payload from config transaction, block number [%d]", blockNumber)
	}

	ctx, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling config which passed initial validity checks")
	}

	txid := committer.ConfigTXPrefix + strconv.FormatUint(ctx.Config.Sequence, 10)
	vc, _, err := c.Vault.Status(txid)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txid)
	}
	switch vc {
	case driver.Valid:
		logger.Infof("config block [%s] already committed, skip it.", txid)
		return nil
	case driver.Unknown:
		logger.Infof("config block [%s] not committed, commit it.", txid)
		// this is okay
	default:
		return errors.Errorf("invalid configtx's [%s] status [%d]", txid, vc)
	}

	var bundle *channelconfig.Bundle
	if c.Resources() == nil {
		// set up the genesis block
		bundle, err = channelconfig.NewBundle(c.ChannelName, ctx.Config, factory.GetDefault())
		if err != nil {
			return errors.Wrapf(err, "failed to build a new bundle")
		}
	} else {
		configTxValidator := c.Resources().ConfigtxValidator()
		err := configTxValidator.Validate(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to validate config transaction, block number [%d]", blockNumber)
		}

		bundle, err = channelconfig.NewBundle(configTxValidator.ChannelID(), ctx.Config, factory.GetDefault())
		if err != nil {
			return errors.Wrapf(err, "failed to create next bundle")
		}

		channelconfig.LogSanityChecks(bundle)
		if err := capabilitiesSupported(bundle); err != nil {
			return err
		}
	}

	if err := c.commitConfig(txid, blockNumber, ctx.Config.Sequence, raw); err != nil {
		return errors.Wrapf(err, "failed committing configtx to the vault")
	}

	return c.ApplyBundle(bundle)
}

// Resources returns the active Channel configuration bundle.
func (c *Channel) Resources() channelconfig.Resources {
	c.ResourcesLock.RLock()
	res := c.ChannelResources
	c.ResourcesLock.RUnlock()
	return res
}

func (c *Channel) commitConfig(txID string, blockNumber uint64, seq uint64, envelope []byte) error {
	logger.Infof("[Channel: %s] commit config transaction number [bn:%d][seq:%d]", c.ChannelName, blockNumber, seq)

	rws, err := c.Vault.NewRWSet(txID)
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
	if err := c.CommitTX(txID, blockNumber, 0, nil); err != nil {
		if err2 := c.DiscardTx(txID, ""); err2 != nil {
			logger.Errorf("failed committing configtx rws [%s]", err2)
		}
		return errors.Wrapf(err, "failed committing configtx rws")
	}
	return nil
}

func (c *Channel) ApplyBundle(bundle *channelconfig.Bundle) error {
	c.ResourcesLock.Lock()
	defer c.ResourcesLock.Unlock()
	c.ChannelResources = bundle

	// update the list of orderers
	ordererConfig, exists := c.ChannelResources.OrdererConfig()
	if exists {
		logger.Debugf("[Channel: %s] Orderer config has changed, updating the list of orderers", c.ChannelName)

		var newOrderers []*grpc.ConnectionConfig
		orgs := ordererConfig.Organizations()
		for _, org := range orgs {
			msp := org.MSP()
			var tlsRootCerts [][]byte
			tlsRootCerts = append(tlsRootCerts, msp.GetTLSRootCerts()...)
			tlsRootCerts = append(tlsRootCerts, msp.GetTLSIntermediateCerts()...)
			for _, endpoint := range org.Endpoints() {
				logger.Debugf("[Channel: %s] Adding orderer endpoint: [%s:%s:%s]", c.ChannelName, org.Name(), org.MSPID(), endpoint)
				// TODO: load from configuration
				newOrderers = append(newOrderers, &grpc.ConnectionConfig{
					Address:           endpoint,
					ConnectionTimeout: 10 * time.Second,
					TLSEnabled:        true,
					TLSRootCertBytes:  tlsRootCerts,
				})
			}
		}
		if len(newOrderers) != 0 {
			logger.Debugf("[Channel: %s] Updating the list of orderers: (%d) found", c.ChannelName, len(newOrderers))
			if err := c.Network.SetConfigOrderers(ordererConfig, newOrderers); err != nil {
				return err
			}
		} else {
			logger.Debugf("[Channel: %s] No orderers found in Channel config", c.ChannelName)
		}
	} else {
		logger.Debugf("no orderer configuration found in Channel config")
	}

	return nil
}

func capabilitiesSupported(res channelconfig.Resources) error {
	ac, ok := res.ApplicationConfig()
	if !ok {
		return errors.Errorf("[Channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] incompatible", res.ConfigtxValidator().ChannelID())
	}

	return nil
}
