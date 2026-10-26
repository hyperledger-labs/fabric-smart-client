/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

const ConfigTXPrefix = "configtx_"

func (c *Committer) HandleConfig(ctx context.Context, _ *common.BlockMetadata, tx CommitTx) (*FinalityEvent, error) {
	logger.Debugf("[%s] Config transaction received: %s", c.ChannelConfig.ID(), tx.TxID)
	if err := c.CommitConfig(ctx, tx.BlkNum, tx.Raw, tx.Envelope); err != nil {
		return nil, errors.Wrapf(err, "cannot commit config envelope for channel [%s]", c.ChannelConfig.ID())
	}
	return &FinalityEvent{Ctx: ctx}, nil
}

func (c *Committer) ReloadConfigTransactions() error {
	ctx, span := c.tracer.Start(context.Background(), "reload_config_transactions")
	defer span.End()

	qe, err := c.Vault.NewQueryExecutor(ctx)
	if err != nil {
		return errors.WithMessage(err, "failed getting query executor")
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

			key, err := compose.CreateCompositeKey(&strings.Builder{}, channelConfigKey, strconv.FormatUint(sequence, 10))
			if err != nil {
				return errors.Wrap(err, "cannot create configtx rws key")
			}
			envelope, err := qe.GetState(ctx, peerNamespace, key)
			if err != nil {
				return errors.Wrap(err, "failed setting configtx state in rws")
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

			sequence++
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
			// A config tx whose commit failed part-way. Block delivery resumes
			// before it, so CommitConfig applies it again.
			c.logger.Debugf("config block at txID [%s] not committed, stop loading", txID)
			done = true
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

// configSequenceOf returns the sequence number carried by a channel
// configuration transaction.
//
// This is the channel's own counter: 0 for the genesis configuration,
// incremented by one by the ordering service for every accepted configuration
// update. It is what the vault entry must be keyed on. The transaction's index
// within its block cannot serve: a configuration block holds exactly one
// transaction, so that index is always 0 and every configuration block would
// collide with the genesis entry.
func configSequenceOf(env *common.Envelope) (uint64, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return 0, errors.Wrap(err, "cannot get payload from config transaction")
	}

	cenv, err := protoutil.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return 0, errors.Wrap(err, "cannot get config envelope from config transaction")
	}

	if cenv.Config == nil {
		return 0, errors.New("config envelope carries no config")
	}

	return cenv.Config.Sequence, nil
}

// CommitConfig applies a channel configuration transaction to this process and
// records it in the vault.
//
// It is safe to call again for a configuration already recorded: that is what a
// retry, a replay after restart, or another replica of the node sharing the
// vault does. The configuration is applied to this process's membership service
// unless it already holds the same or a later sequence.
func (c *Committer) CommitConfig(ctx context.Context, blockNumber driver.BlockNum, raw []byte, env *common.Envelope) error {
	logger.DebugfContext(ctx, "Start commit config")
	defer logger.DebugfContext(ctx, "End commit config")
	commitConfigMutex.Lock()
	defer commitConfigMutex.Unlock()

	if env == nil {
		return errors.New("envelope nil")
	}

	sequence, err := configSequenceOf(env)
	if err != nil {
		return errors.Wrap(err, "cannot read the config sequence")
	}

	// first we check if config is already committed
	txID := ConfigTXPrefix + strconv.FormatUint(sequence, 10)
	vc, _, err := c.Vault.Status(ctx, txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		// Every replica of a node shares the vault, so Valid means some replica
		// applied this configuration, not necessarily this process. Skip it only
		// if this process already holds it or a later one: re-applying an older
		// configuration would roll the membership service back. A held sequence
		// stands for a fully applied one: applyConfigUpdates fails only on a
		// configuration no replica could apply, so none would have recorded it.
		if current, err := c.MembershipService.ConfigSequence(); err == nil && current >= sequence {
			c.logger.Debugf("config block [%s] already committed, skip it.", txID)
			return nil
		}
		c.logger.Debugf("config block [%s] committed by another replica, apply it.", txID)
		return c.applyConfig(env, txID)
	case driver.Unknown, driver.Busy:
		// Busy is what an attempt that failed after opening the config tx's rwset
		// leaves behind, or another replica's attempt in progress; either way this
		// process applies it and records it. commitConfigMutex rules out a
		// concurrent attempt in this process only.
		c.logger.Debugf("config block [%s] not committed [%d], continue.", txID, vc)
	default:
		return errors.Errorf("invalid configtx's [%s] status [%d]", txID, vc)
	}

	// Applied before the vault write, so a Valid entry means "in force on the
	// replica that wrote it" rather than "stored". Writing first would let a
	// failed apply leave a Valid entry that no process has acted on.
	if err := c.applyConfig(env, txID); err != nil {
		return err
	}

	// Recorded last: a crash before this replays the config on restart, which is
	// idempotent. Skipping one that was never applied is not.
	if err := c.applyConfigCommit(ctx, txID, blockNumber, sequence, raw); err != nil {
		// The entry may be Valid anyway: another replica sharing the vault
		// recorded it in the meantime, or this write landed before the
		// connection broke. The configuration is applied here, so that is success.
		if vc, _, serr := c.Vault.Status(ctx, txID); serr == nil && vc == driver.Valid {
			c.logger.Debugf("config block [%s] already recorded.", txID)
			return nil
		}
		return errors.Wrap(err, "failed committing configtx to the vault")
	}

	return nil
}

// applyConfig installs env in the membership service and reconfigures the
// ordering service from it.
func (c *Committer) applyConfig(env *common.Envelope, txID string) error {
	if err := c.MembershipService.Update(env); err != nil {
		return errors.Wrapf(err, "failed updating membership service for configtx [%s]", txID)
	}

	if err := c.applyConfigUpdates(); err != nil {
		return errors.Wrapf(err, "failed applying config updates for configtx [%s]", txID)
	}

	return nil
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
