/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

func (c *Channel) Status(txID string) (driver.ValidationCode, string, []string, error) {
	vc, message, err := c.Vault.Status(txID)
	if err != nil {
		logger.Errorf("failed to get status of [%s]: %s", txID, err)
		return driver.Unknown, "", nil, err
	}
	if vc == driver.Unknown {
		// give it a second chance
		if c.EnvelopeService().Exists(txID) {
			if err := c.extractStoredEnvelopeToVault(txID); err != nil {
				return driver.Unknown, "", nil, errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
			vc = driver.Busy
		} else {
			// check status reporter, if any
			for _, reporter := range c.StatusReporters {
				externalStatus, externalMessage, _, err := reporter.Status(txID)
				if err == nil && externalStatus != driver.Unknown {
					vc = externalStatus
					message = externalMessage
				}
			}
		}
	}
	return vc, message, nil, nil
}

func (c *Channel) ProcessNamespace(nss ...string) error {
	c.ProcessNamespaces = append(c.ProcessNamespaces, nss...)
	return nil
}

func (c *Channel) GetProcessNamespace() []string {
	return c.ProcessNamespaces
}

func (c *Channel) AddStatusReporter(sr driver.StatusReporter) error {
	c.StatusReporters = append(c.StatusReporters, sr)
	return nil
}

func (c *Channel) DiscardTx(txID string, message string) error {
	logger.Debugf("discarding transaction [%s] with message [%s]", txID, message)

	defer c.notifyTxStatus(txID, driver.Invalid, message)
	vc, _, deps, err := c.Status(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	if vc == driver.Unknown {
		// give it a second chance
		if c.EnvelopeService().Exists(txID) {
			if err := c.extractStoredEnvelopeToVault(txID); err != nil {
				return errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
		} else {
			// check status reporter, if any
			found := false
			for _, reporter := range c.StatusReporters {
				externalStatus, _, _, err := reporter.Status(txID)
				if err == nil && externalStatus != driver.Unknown {
					found = true
					break
				}
			}
			if !found {
				logger.Debugf("Discarding transaction [%s] skipped, tx is unknown", txID)
				return nil
			}
		}
	}

	if err := c.Vault.DiscardTx(txID, message); err != nil {
		logger.Errorf("failed discarding tx [%s] in vault: %s", txID, err)
	}
	for _, dep := range deps {
		if err := c.Vault.DiscardTx(dep, message); err != nil {
			logger.Errorf("failed discarding dependant tx [%s] of [%s] in vault: %s", dep, txID, err)
		}
	}
	return nil
}

func (c *Channel) CommitTX(txID string, block uint64, indexInBlock int, envelope *common.Envelope) (err error) {
	logger.Debugf("Committing transaction [%s,%d,%d]", txID, block, indexInBlock)
	defer logger.Debugf("Committing transaction [%s,%d,%d] done [%s]", txID, block, indexInBlock, err)
	defer func() {
		if err == nil {
			c.notifyTxStatus(txID, driver.Valid, "")
		}
	}()

	vc, _, _, err := c.Status(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		// This should generate a panic
		logger.Debugf("[%s] is already valid", txID)
		return errors.Errorf("[%s] is already valid", txID)
	case driver.Invalid:
		// This should generate a panic
		logger.Debugf("[%s] is invalid", txID)
		return errors.Errorf("[%s] is invalid", txID)
	case driver.Unknown:
		return c.commitUnknown(txID, block, indexInBlock, envelope)
	case driver.Busy:
		return c.commit(txID, block, indexInBlock, envelope)
	default:
		return errors.Errorf("invalid status code [%d] for [%s]", vc, txID)
	}
}

func (c *Channel) SubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	_, topic := compose.CreateTxTopic(c.Network.Name(), c.ChannelName, txID)
	l := &TxEventsListener{listener: listener}
	logger.Debugf("[%s] Subscribing to transaction status changes", txID)
	c.EventsSubscriber.Subscribe(topic, l)
	logger.Debugf("[%s] store mapping", txID)
	c.Subscribers.Set(topic, listener, l)
	logger.Debugf("[%s] Subscribing to transaction status changes done", txID)
	return nil
}

func (c *Channel) UnsubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	_, topic := compose.CreateTxTopic(c.Network.Name(), c.ChannelName, txID)
	l, ok := c.Subscribers.Get(topic, listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	el, ok := l.(events.Listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	c.Subscribers.Delete(topic, listener)
	c.EventsSubscriber.Unsubscribe(topic, el)
	return nil
}

func (c *Channel) commitUnknown(txID string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	// if an envelope exists for the passed txID, then commit it
	if c.EnvelopeService().Exists(txID) {
		return c.commitStoredEnvelope(txID, block, indexInBlock)
	}

	var envelopeRaw []byte
	var err error
	if envelope != nil {
		// Store it
		envelopeRaw, err = proto.Marshal(envelope)
		if err != nil {
			return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
		}
	} else {
		// fetch envelope and store it
		envelopeRaw, err = c.FetchEnvelope(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed getting rwset for tx [%s]", txID)
		}
	}

	// shall we commit this unknown envelope
	if ok, err := c.filterUnknownEnvelope(txID, envelopeRaw); err != nil || !ok {
		logger.Debugf("[%s] unknown envelope will not be processed [%b,%s]", txID, ok, err)
		return nil
	}

	if err := c.EnvelopeService().StoreEnvelope(txID, envelopeRaw); err != nil {
		return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
	}
	rws, _, err := c.RWSetLoader.GetRWSetFromEvn(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	rws.Done()
	return c.commit(txID, block, indexInBlock, envelope)
}

func (c *Channel) filterUnknownEnvelope(txID string, envelope []byte) (bool, error) {
	rws, _, err := c.RWSetLoader.GetInspectingRWSetFromEvn(txID, envelope)
	if err != nil {
		return false, errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	defer rws.Done()

	logger.Debugf("[%s] contains namespaces [%v] or `initialized` key", txID, rws.Namespaces())
	for _, ns := range rws.Namespaces() {
		for _, namespace := range c.ProcessNamespaces {
			if namespace == ns {
				logger.Debugf("[%s] contains namespaces [%v], select it", txID, rws.Namespaces())
				return true, nil
			}
		}

		// search a read dependency on a key containing "initialized"
		for pos := 0; pos < rws.NumReads(ns); pos++ {
			k, err := rws.GetReadKeyAt(ns, pos)
			if err != nil {
				return false, errors.WithMessagef(err, "Error reading key at [%d]", pos)
			}
			if strings.Contains(k, "initialized") {
				logger.Debugf("[%s] contains 'initialized' key [%v] in [%s], select it", txID, ns, rws.Namespaces())
				return true, nil
			}
		}
	}

	status, _, _, _ := c.Status(txID)
	return status == driver.Busy, nil
}

func (c *Channel) commitStoredEnvelope(txID string, block uint64, indexInBlock int) error {
	logger.Debugf("found envelope for transaction [%s], committing it...", txID)
	if err := c.extractStoredEnvelopeToVault(txID); err != nil {
		return err
	}
	// commit
	return c.commitLocal(txID, block, indexInBlock, nil)
}

func (c *Channel) extractStoredEnvelopeToVault(txID string) error {
	rws, _, err := c.RWSetLoader.GetRWSetFromEvn(txID)
	if err != nil {
		// If another replica of the same node created the RWSet
		rws, _, err = c.RWSetLoader.GetRWSetFromETx(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed to extract rws from envelope and etx [%s]", txID)
		}
	}
	rws.Done()
	return nil
}

func (c *Channel) commit(txID string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	logger.Debugf("[%s] is known.", txID)
	if err := c.commitLocal(txID, block, indexInBlock, envelope); err != nil {
		return err
	}
	return nil
}

func (c *Channel) commitLocal(txID string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	// This is a normal transaction, validated by Fabric.
	// Commit it cause Fabric says it is valid.
	logger.Debugf("[%s] committing", txID)

	// Match rwsets if envelope is not empty
	if envelope != nil {
		logger.Debugf("[%s] matching rwsets", txID)

		pt, headerType, err := newProcessedTransactionFromEnvelope(envelope)
		if err != nil && headerType == -1 {
			logger.Errorf("[%s] failed to unmarshal envelope [%s]", txID, err)
			return err
		}
		if headerType == int32(common.HeaderType_ENDORSER_TRANSACTION) {
			if !c.Vault.RWSExists(txID) && c.EnvelopeService().Exists(txID) {
				// Then match rwsets
				if err := c.extractStoredEnvelopeToVault(txID); err != nil {
					return errors.WithMessagef(err, "failed to load stored enveloper into the vault")
				}
				if err := c.Vault.Match(txID, pt.Results()); err != nil {
					logger.Errorf("[%s] rwsets do not match [%s]", txID, err)
					return errors.Wrapf(committer.ErrDiscardTX, "[%s] rwsets do not match [%s]", txID, err)
				}
			} else {
				// Store it
				envelopeRaw, err := proto.Marshal(envelope)
				if err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				if err := c.EnvelopeService().StoreEnvelope(txID, envelopeRaw); err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				rws, _, err := c.RWSetLoader.GetRWSetFromEvn(txID)
				if err != nil {
					return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
				}
				rws.Done()
			}
		}
	}

	// Post-Processes
	logger.Debugf("[%s] post process rwset", txID)

	if err := c.postProcessTx(txID); err != nil {
		// This should generate a panic
		return err
	}

	// Commit
	logger.Debugf("[%s] commit in vault", txID)
	if err := c.Vault.CommitTX(txID, block, indexInBlock); err != nil {
		// This should generate a panic
		return err
	}

	return nil
}

func (c *Channel) postProcessTx(txID string) error {
	if err := c.Network.ProcessorManager().ProcessByID(c.ChannelName, txID); err != nil {
		// This should generate a panic
		return err
	}
	return nil
}

func (c *Channel) notifyTxStatus(txID string, vc driver.ValidationCode, message string) {
	// We publish two events here:
	// 1. The first will be caught by the listeners that are listening for any transaction id.
	// 2. The second will be caught by the listeners that are listening for the specific transaction id.
	sb, topic := compose.CreateTxTopic(c.Network.Name(), c.ChannelName, "")
	c.EventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic:         topic,
		TxID:              txID,
		VC:                vc,
		ValidationMessage: message,
	})
	c.EventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic:         compose.AppendAttributesOrPanic(sb, txID),
		TxID:              txID,
		VC:                vc,
		ValidationMessage: message,
	})
}

type TxEventsListener struct {
	listener driver.TxStatusChangeListener
}

func (l *TxEventsListener) OnReceive(event events.Event) {
	tsc := event.Message().(*driver.TransactionStatusChanged)
	if err := l.listener.OnStatusChange(tsc.TxID, int(tsc.VC), tsc.ValidationMessage); err != nil {
		logger.Errorf("failed to notify listener for tx [%s] with err [%s]", tsc.TxID, err)
	}
}
