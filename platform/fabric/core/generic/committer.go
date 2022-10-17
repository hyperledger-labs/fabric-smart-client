/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

func (c *channel) Status(txid string) (driver.ValidationCode, []string, error) {
	vc, err := c.vault.Status(txid)
	if err != nil {
		logger.Errorf("failed to get status of [%s]: %s", txid, err)
		return driver.Unknown, nil, err
	}
	if c.externalCommitter == nil {
		return vc, nil, nil
	}

	_, dependantTxIDs, _, err := c.externalCommitter.Status(txid)
	if err != nil {
		logger.Errorf("failed to get external status of [%s]: %s", txid, err)
		return driver.Unknown, nil, err
	}
	if vc == driver.Unknown && len(dependantTxIDs) != 0 {
		return driver.HasDependencies, dependantTxIDs, nil
	}
	return vc, dependantTxIDs, nil
}

func (c *channel) ProcessNamespace(nss ...string) error {
	c.processNamespaces = append(c.processNamespaces, nss...)
	return nil
}

func (c *channel) GetProcessNamespace() []string {
	return c.processNamespaces
}

func (c *channel) DiscardTx(txid string) error {
	logger.Debugf("Discarding transaction [%s]", txid)

	defer c.notifyTxStatus(txid, driver.Invalid)
	vc, deps, err := c.Status(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txid)
	}
	if vc == driver.Unknown {
		logger.Debugf("Discarding transaction [%s] skipped, tx is unknown", txid)
		return nil
	}

	if err := c.vault.DiscardTx(txid); err != nil {
		logger.Errorf("failed discarding tx [%s] in vault: %s", txid, err)
	}
	for _, dep := range deps {
		if err := c.vault.DiscardTx(dep); err != nil {
			logger.Errorf("failed discarding dependant tx [%s] of [%s] in vault: %s", dep, txid, err)
		}
	}
	return nil
}

func (c *channel) CommitTX(txid string, block uint64, indexInBlock int, envelope *common.Envelope) (err error) {
	logger.Debugf("Committing transaction [%s,%d,%d]", txid, block, indexInBlock)
	defer logger.Debugf("Committing transaction [%s,%d,%d] done [%s]", txid, block, indexInBlock, err)
	defer func() {
		if err == nil {
			c.notifyTxStatus(txid, driver.Valid)
		}
	}()

	vc, deps, err := c.Status(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txid)
	}
	switch vc {
	case driver.Valid:
		// This should generate a panic
		logger.Debugf("[%s] is already valid", txid)
		return errors.Errorf("[%s] is already valid", txid)
	case driver.Invalid:
		// This should generate a panic
		logger.Debugf("[%s] is invalid", txid)
		return errors.Errorf("[%s] is invalid", txid)
	case driver.Unknown:
		return c.commitUnknown(txid, block, indexInBlock)
	case driver.HasDependencies:
		return c.commitDeps(txid, block, indexInBlock)
	case driver.Busy:
		return c.commit(txid, deps, block, indexInBlock, envelope)
	default:
		return errors.Errorf("invalid status code [%d] for [%s]", vc, txid)
	}
}

func (c *channel) commitUnknown(txID string, block uint64, indexInBlock int) error {
	// if an envelope exists for the passed txID, then commit it
	if c.EnvelopeService().Exists(txID) {
		return c.commitStoredEnvelope(txID, block, indexInBlock)
	}

	// process the transaction if it contains given namespaces
	if len(c.processNamespaces) == 0 {
		// This should be ignored
		logger.Debugf("[%s] is unknown and will be ignored", txID)
		return nil
	}

	logger.Debugf("[%s] is unknown but will be processed for known namespaces", txID)
	pt, err := c.GetTransactionByID(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed fetching tx [%s]", txID)
	}
	if !pt.IsValid() {
		return errors.Errorf("fetched tx [%s] should have been valid, instead it is [%s]", txID, pb.TxValidationCode_name[pt.ValidationCode()])
	}

	rws, err := c.GetRWSet(txID, pt.Results())
	if err != nil {
		return errors.WithMessagef(err, "failed getting rwset for tx [%s]", txID)
	}
	found := false
	logger.Debugf("[%s] contains namespaces [%v]", txID, rws.Namespaces())
	for _, ns := range rws.Namespaces() {
		for _, pns := range c.processNamespaces {
			if ns == pns {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	rws.Done()

	if !found {
		logger.Debugf("[%s] no known namespaces found", txID)
		// nothing to commit
		return nil
	}

	// commit this transaction because it contains one of the namespaces to be processed anyway
	logger.Debugf("[%s] known namespaces found, commit", txID)
	return c.commit(txID, nil, block, indexInBlock, nil)
}

func (c *channel) commitStoredEnvelope(txID string, block uint64, indexInBlock int) error {
	logger.Debugf("found envelope for transaction [%s], committing it...", txID)
	if err := c.extractStoredEnvelopeToVault(txID); err != nil {
		return err
	}
	// commit
	return c.commitLocal(txID, block, indexInBlock, nil)
}

func (c *channel) extractStoredEnvelopeToVault(txID string) error {
	// extract envelope
	envRaw, err := c.EnvelopeService().LoadEnvelope(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed to load fabric envelope for [%s]", txID)
	}
	env, err := protoutil.UnmarshalEnvelope(envRaw)
	if err != nil {
		return errors.WithMessagef(err, "failed to unmarshal fabric envelope for [%s][%s]", txID, hash.Hashable(envRaw).String())
	}
	pt, err := newProcessedTransactionFromEnvelope(env)
	if err != nil {
		return errors.WithMessagef(err, "failed to parse fabric envelope for [%s][%s]", txID, hash.Hashable(envRaw).String())
	}
	// load into the vault
	rws, err := c.vault.GetRWSet(txID, pt.Results())
	if err != nil {
		return errors.WithMessagef(err, "failed to parse fabric envelope's rws for [%s][%s]", txID, hash.Hashable(envRaw).String())
	}
	rws.Done()
	return nil
}

func (c *channel) commit(txid string, deps []string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	logger.Debugf("[%s] is known.", txid)

	switch {
	case len(deps) != 0:
		if err := c.commitExternal(txid, block, indexInBlock); err != nil {
			return err
		}
	default:
		if err := c.commitLocal(txid, block, indexInBlock, envelope); err != nil {
			return err
		}
	}
	return nil
}

func (c *channel) commitDeps(txid string, block uint64, indexInBlock int) error {
	// This should not generate a panic if the transaction is deemed invalid
	logger.Debugf("[%s] is unknown but have dependencies, commit as multi-shard pvt", txid)

	// Validate and commit
	vc, err := c.externalCommitter.Validate(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed validating transaction [%s]", txid)
	}
	switch vc {
	case driver.Valid:
		if err := c.externalCommitter.CommitTX(txid, block, indexInBlock); err != nil {
			return errors.WithMessagef(err, "failed committing tx [%s]", txid)
		}
		return nil
	case driver.Invalid:
		if err := c.externalCommitter.DiscardTX(txid); err != nil {
			logger.Errorf("failed committing tx [%s] with err [%s]", txid, err)
		}
		return nil
	}
	return nil
}

func (c *channel) commitExternal(txid string, block uint64, indexInBlock int) error {
	logger.Debugf("[%s] Committing as multi-shard pvt.", txid)

	// Ask for finality
	_, _, parties, err := c.externalCommitter.Status(txid)
	if err != nil {
		return errors.Wrapf(err, "failed getting parties for [%s]", txid)
	}
	if err := c.IsFinalForParties(txid, parties...); err != nil {
		return err
	}

	// Validate and commit
	vc, err := c.externalCommitter.Validate(txid)
	if err != nil {
		return errors.WithMessagef(err, "failed validating transaction [%s]", txid)
	}
	switch vc {
	case driver.Valid:
		if err := c.externalCommitter.CommitTX(txid, block, indexInBlock); err != nil {
			return errors.WithMessagef(err, "failed committing tx [%s]", txid)
		}
		return nil
	case driver.Invalid:
		if err := c.externalCommitter.DiscardTX(txid); err != nil {
			logger.Errorf("failed committing tx [%s] with err [%s]", txid, err)
		}
		return nil
	}
	return nil
}

func (c *channel) commitLocal(txid string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	// This is a normal transaction, validated by Fabric.
	// Commit it cause Fabric says it is valid.
	logger.Debugf("[%s] Committing", txid)

	// Match rwsets if envelope is not empty
	if envelope != nil {
		logger.Debugf("[%s] Matching rwsets", txid)

		pt, err := newProcessedTransactionFromEnvelope(envelope)
		if err != nil {
			logger.Error("[%s] failed to unmarshal envelope [%s]", txid, err)
			return err
		}

		if !c.vault.RWSExists(txid) {
			if err := c.extractStoredEnvelopeToVault(txid); err != nil {
				return errors.WithMessagef(err, "failed to load stored enveloper into the vault")
			}
		}

		if err := c.vault.Match(txid, pt.Results()); err != nil {
			logger.Error("[%s] rwsets do not match [%s]", txid, err)
			return err
		}

	}

	// Post-Processes
	logger.Debugf("[%s] Post Processes", txid)

	if err := c.postProcessTx(txid); err != nil {
		// This should generate a panic
		return err
	}

	// Commit
	logger.Debugf("[%s] Commit in vault", txid)
	if err := c.vault.CommitTX(txid, block, indexInBlock); err != nil {
		// This should generate a panic
		return err
	}

	return nil
}

func (c *channel) postProcessTx(txid string) error {
	if err := c.network.ProcessorManager().ProcessByID(c.name, txid); err != nil {
		// This should generate a panic
		return err
	}
	return nil
}

// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *channel) SubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	_, topic := compose.CreateTxTopic(c.network.Name(), c.name, txID)
	l := &TxEventsListener{listener: listener}
	logger.Debugf("[%s] Subscribing to transaction status changes", txID)
	c.eventsSubscriber.Subscribe(topic, l)
	logger.Debugf("[%s] store mapping", txID)
	c.subscribers.Set(topic, listener, l)
	logger.Debugf("[%s] Subscribing to transaction status changes done", txID)
	return nil
}

// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *channel) UnsubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	_, topic := compose.CreateTxTopic(c.network.Name(), c.name, txID)
	l, ok := c.subscribers.Get(topic, listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	el, ok := l.(events.Listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	c.subscribers.Delete(topic, listener)
	c.eventsSubscriber.Unsubscribe(topic, el)
	return nil
}

func (c *channel) notifyTxStatus(txID string, vc driver.ValidationCode) {
	// We publish two events here:
	// 1. The first will be caught by the listeners that are listening for any transaction id.
	// 2. The second will be caught by the listeners that are listening for the specific transaction id.
	sb, topic := compose.CreateTxTopic(c.network.Name(), c.name, "")
	c.eventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic: topic,
		TxID:      txID,
		VC:        vc,
	})
	c.eventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic: compose.AppendAttributesOrPanic(sb, txID),
		TxID:      txID,
		VC:        vc,
	})
}

type TxEventsListener struct {
	listener driver.TxStatusChangeListener
}

func (l *TxEventsListener) OnReceive(event events.Event) {
	tsc := event.Message().(*driver.TransactionStatusChanged)
	if err := l.listener.OnStatusChange(tsc.TxID, int(tsc.VC)); err != nil {
		logger.Errorf("failed to notify listener for tx [%s] with err [%s]", tsc.TxID, err)
	}
}
