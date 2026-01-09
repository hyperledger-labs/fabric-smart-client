/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"strconv"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
)

const ConfigTXPrefix = "configtx_"

func (c *Committer) HandleConfig(ctx context.Context, _ *common.BlockMetadata, tx CommitTx) (*FinalityEvent, error) {
	logger.Debugf("[%s] Config transaction received: %s", c.ChannelConfig.ID(), tx.TxID)
	if err := c.CommitConfig(ctx, tx.BlkNum, tx.TxNum, tx.Raw, tx.Envelope); err != nil {
		return nil, errors.Wrapf(err, "cannot commit config envelope for channel [%s]", c.ChannelConfig.ID())
	}
	return &FinalityEvent{Ctx: ctx}, nil
}

func (c *Committer) ReloadConfigTransactions() error {
	ctx, span := c.tracer.Start(context.Background(), "reload_config_transactions")
	defer span.End()

	qe, err := c.Vault.NewQueryExecutor(ctx)
	if err != nil {
		return errors.WithMessagef(err, "failed getting query executor")
	}
	defer func() {
		if err := qe.Done(); err != nil {
			logger.Errorf("error closing query executor: %v", err)
		}
	}()

	c.logger.Debugf("looking up the latest config block available")
	var sequence uint64 = 0
	for {
		txID := ConfigTXPrefix + strconv.FormatUint(sequence, 10)
		vc, _, err := c.Vault.Status(ctx, txID)
		if err != nil {
			return errors.WithMessagef(err, "failed getting tx's status [%s]", txID)
		}
		c.logger.Debugf("check config block at txID [%s], status [%v]...", txID, vc)
		done := false
		switch vc {
		case driver.Valid:
			c.logger.Debugf("config block available, txID [%s], loading...", txID)

			key, err := rwset.CreateCompositeKey(channelConfigKey, []string{strconv.FormatUint(sequence, 10)})
			if err != nil {
				return errors.Wrapf(err, "cannot create configtx rws key")
			}
			envelope, err := qe.GetState(ctx, peerNamespace, key)
			if err != nil {
				return errors.Wrapf(err, "failed setting configtx state in rws")
			}
			env, err := protoutil.UnmarshalEnvelope(envelope.Raw)
			if err != nil {
				return errors.Wrapf(err, "cannot get payload from config transaction [%s]", txID)
			}

			if err := c.MembershipService.Update(env); err != nil {
				return err
			}

			if err := c.applyConfigUpdates(); err != nil {
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

			c.logger.Debugf("config block at txID [%s] unavailable, stop loading", txID)
			done = true
		case driver.Busy:
			c.logger.Debugf("someone else is modifying it. retry...")
			time.Sleep(1 * time.Second)
			continue
		default:
			return errors.Errorf("invalid configtx's [%s] status [%d]", txID, vc)
		}
		if done {
			c.logger.Debugf("loading config block done")
			break
		}
	}
	if sequence == 1 {
		c.logger.Debugf("no config block available, must start from genesis")
		// no configuration block found
		return nil
	}
	c.logger.Debugf("latest config block available at sequence [%d]", sequence-1)

	return nil
}

// CommitConfig is used to validate and apply configuration transactions for a Channel.
func (c *Committer) CommitConfig(ctx context.Context, blockNumber driver.BlockNum, txNum driver.TxNum, raw []byte, env *common.Envelope) error {
	logger.DebugfContext(ctx, "Start commit config")
	defer logger.DebugfContext(ctx, "End commit config")
	commitConfigMutex.Lock()
	defer commitConfigMutex.Unlock()

	if env == nil {
		return errors.Errorf("envelope nil")
	}

	// first we check if config is already committed
	txID := ConfigTXPrefix + strconv.FormatUint(txNum, 10)
	vc, _, err := c.Vault.Status(ctx, txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		c.logger.Debugf("config block [%s] already committed, skip it.", txID)
		return nil
	case driver.Unknown:
		c.logger.Debugf("config block [%s] not committed, continue.", txID)
		// this is okay
	default:
		return errors.Errorf("invalid configtx's [%s] status [%d]", txID, vc)
	}

	// validate config as a dry update of the membership service
	if err := c.MembershipService.DryUpdate(env); err != nil {
		return errors.Wrapf(err, "config update error, block number [%d]", blockNumber)
	}

	// when validation passes, we can commit the config transaction
	if err := c.commitConfig(ctx, txID, blockNumber, txNum, raw); err != nil {
		return errors.Wrapf(err, "failed committing configtx to the vault")
	}

	// once committed, we can update the membership service
	if err := c.MembershipService.Update(env); err != nil {
		// this should not have happened
		panic(err)
	}

	// and apply other updates
	return c.applyConfigUpdates()
}

// applyConfigUpdates notifies all components that are impacted by a config update
func (c *Committer) applyConfigUpdates() error {
	// update the list of orderers
	consensusType, endpoints, err := c.MembershipService.OrdererConfig(c.ConfigService)
	if err != nil || len(endpoints) == 0 {
		c.logger.Debugf("[Channel: %s] No orderers found in Channel config", c.ChannelConfig.ID())
		return err
	}

	c.logger.Debugf("[Channel: %s] Updating the list of orderers: (%d) found", c.ChannelConfig.ID(), len(endpoints))
	return c.OrderingService.Configure(consensusType, endpoints)
}
