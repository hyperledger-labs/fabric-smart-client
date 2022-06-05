/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"fmt"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
	"strings"
	"sync"
	"time"
)

var logger = flogging.MustGetLogger("orion-sdk.committer")

type Finality interface {
	IsFinal(txID string, address string) error
}

type Vault interface {
	Status(txID string) (driver.ValidationCode, error)
	DiscardTx(txid string) error
	CommitTX(txid string, block uint64, indexInBloc int) error
}

type ProcessorManager interface {
	ProcessByID(txid string) error
}

type committer struct {
	networkName         string
	vault               Vault
	finality            Finality
	pm                  ProcessorManager
	waitForEventTimeout time.Duration

	quietNotifier bool

	listeners      map[string][]chan TxEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration

	eventsSubscriber events.Subscriber
	eventsPublisher  events.Publisher
	subscribers      *events.Subscribers
}

func New(
	networkName string,
	pm ProcessorManager,
	vault Vault,
	finality Finality,
	waitForEventTimeout time.Duration,
	quiet bool,
	eventsPublisher events.Publisher,
	eventsSubscriber events.Subscriber,
) (*committer, error) {
	d := &committer{
		networkName:         networkName,
		vault:               vault,
		waitForEventTimeout: waitForEventTimeout,
		quietNotifier:       quiet,
		listeners:           map[string][]chan TxEvent{},
		mutex:               sync.Mutex{},
		finality:            finality,
		pm:                  pm,
		pollingTimeout:      100 * time.Millisecond,
		eventsSubscriber:    eventsSubscriber,
		eventsPublisher:     eventsPublisher,
		subscribers:         events.NewSubscribers(),
	}
	return d, nil
}

// Commit commits the transactions in the block passed as argument
func (c *committer) Commit(block *types.AugmentedBlockHeader) error {
	bn := block.Header.BaseHeader.Number
	for i, txID := range block.TxIds {
		var event TxEvent
		event.Txid = txID
		event.Block = block.Header.BaseHeader.Number
		event.IndexInBlock = i

		switch block.Header.ValidationInfo[i].Flag {
		case types.Flag_VALID:
			// if is already committed, do nothing
			vc, err := c.vault.Status(txID)
			if err != nil {
				return errors.Wrapf(err, "failed to get status of tx %s", txID)
			}
			switch vc {
			case driver.Valid:
				logger.Debugf("tx %s is already committed", txID)
				continue
			case driver.Invalid:
				logger.Debugf("tx %s is already invalid", txID)
				return errors.Errorf("tx %s is already invalid but it is marked as valid by orion", txID)
			case driver.Unknown:
				logger.Debugf("tx %s is unknown, ignore it", txID)
				continue
			}

			// post process
			if err := c.pm.ProcessByID(txID); err != nil {
				return errors.Wrapf(err, "failed to process tx %s", txID)
			}

			// commit
			if err := c.vault.CommitTX(txID, bn, i); err != nil {
				return errors.WithMessagef(err, "failed to commit tx %s", txID)
			}
			event.Committed = true
		default:
			// rollback
			if err := c.vault.DiscardTx(txID); err != nil {
				return errors.WithMessagef(err, "failed to discard tx %s", txID)
			}
		}
		c.notifyFinality(event)
	}
	return nil
}

// IsFinal takes in input a transaction id and waits for its confirmation.
func (c *committer) IsFinal(txid string) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final?", txid)
	}

	for iter := 0; iter < 3; iter++ {
		vd, err := c.vault.Status(txid)
		if err == nil {
			switch vd {
			case driver.Valid:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is valid", txid)
				}
				return nil
			case driver.Invalid:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is not valid", txid)
				}
				return errors.Errorf("transaction [%s] is not valid", txid)
			case driver.Busy:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is known", txid)
				}
			case driver.Unknown:
				if iter >= 2 {
					return errors.Errorf("transaction [%s] is unknown", txid)
				}
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is unknown with no deps, wait a bit and retry [%d]", txid, iter)
				}
				time.Sleep(100 * time.Millisecond)
			default:
				panic(fmt.Sprintf("invalid status code, got %c", vd))
			}
		} else {
			logger.Errorf("Is [%s] final? Failed getting transaction status from vault", txid)
			return errors.WithMessagef(err, "failed getting transaction status from vault [%s]", txid)
		}
	}

	// Listen to the event
	return c.listenToFinality(txid, c.waitForEventTimeout)
}

// SubscribeTxStatusChanges registers a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *committer) SubscribeTxStatusChanges(txID string, wrapped driver.TxStatusChangeListener) error {
	logger.Debugf("Subscribing to tx status changes for [%s]", txID)
	var sb strings.Builder
	sb.WriteString("tx")
	sb.WriteString(c.networkName)
	sb.WriteString(txID)
	wrapper := &TxEventsListener{listener: wrapped}
	c.eventsSubscriber.Subscribe(sb.String(), wrapper)
	c.subscribers.Set(txID, wrapped, wrapper)
	logger.Debugf("Subscribed to tx status changes for [%s] done", txID)
	return nil
}

// UnsubscribeTxStatusChanges unregisters a listener for transaction status changes for the passed transaction id.
// If the transaction id is empty, the listener will be called for all transactions.
func (c *committer) UnsubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	var sb strings.Builder
	sb.WriteString("tx")
	sb.WriteString(c.networkName)
	sb.WriteString(txID)
	l, ok := c.subscribers.Get(txID, listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	el, ok := l.(events.Listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	c.eventsSubscriber.Unsubscribe(sb.String(), el)
	return nil
}

func (c *committer) notifyTxStatus(txID string, vc driver.ValidationCode) {
	// We publish two events here:
	// 1. The first will be caught by the listeners that are listening for any transaction id.
	// 2. The second will be caught by the listeners that are listening for the specific transaction id.
	var sb strings.Builder
	c.eventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic: CreateCompositeKeyOrPanic(&sb, "tx", c.networkName, txID),
		TxID:      txID,
		VC:        vc,
	})
	sb.WriteString(txID)
	c.eventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic: AppendAttributesOrPanic(&sb, txID),
		TxID:      txID,
		VC:        vc,
	})
}

func (c *committer) addFinalityListener(txid string, ch chan TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.listeners[txid]
	if !ok {
		ls = []chan TxEvent{}
		c.listeners[txid] = ls
	}
	ls = append(ls, ch)
	c.listeners[txid] = ls
}

func (c *committer) deleteFinalityListener(txid string, ch chan TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.listeners[txid]
	if !ok {
		return
	}
	for i, l := range ls {
		if l == ch {
			ls = append(ls[:i], ls[i+1:]...)
			c.listeners[txid] = ls
			return
		}
	}
}

func (c *committer) notifyFinality(event TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if event.Committed {
		c.notifyTxStatus(event.Txid, driver.Valid)
	} else {
		c.notifyTxStatus(event.Txid, driver.Invalid)
	}

	if event.Err != nil && !c.quietNotifier {
		logger.Warningf("An error occurred for tx [%s], event: [%v]", event.Txid, event)
	}

	listeners := c.listeners[event.Txid]
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Notify the finality of [%s] to [%d] listeners, event: [%v]", event.Txid, len(listeners), event)
	}
	for _, listener := range listeners {
		listener <- event
	}

	for _, txid := range event.DependantTxIDs {
		listeners := c.listeners[txid]
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("Notify the finality of [%s] (dependant) to [%d] listeners, event: [%v]", txid, len(listeners), event)
		}
		for _, listener := range listeners {
			listener <- event
		}
	}
}

func (c *committer) listenToFinality(txID string, timeout time.Duration) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Listen to finality of [%s]", txID)
	}

	// notice that adding the listener can happen after the event we are looking for has already happened
	// therefore we need to check more often before the timeout happens
	ch := make(chan TxEvent, 100)
	c.addFinalityListener(txID, ch)
	defer c.deleteFinalityListener(txID, ch)

	iterations := int(timeout.Milliseconds() / c.pollingTimeout.Milliseconds())
	if iterations == 0 {
		iterations = 1
	}
	for i := 0; i < iterations; i++ {
		select {
		case event := <-ch:
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Got an answer to finality of [%s]: [%s]", txID, event.Err)
			}
			return event.Err
		case <-time.After(c.pollingTimeout):
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Got a timeout for finality of [%s], check the status", txID)
			}
			vd, err := c.vault.Status(txID)
			if err == nil {
				switch vd {
				case driver.Valid:
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("Listen to finality of [%s]. VALID", txID)
					}
					return nil
				case driver.Invalid:
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("Listen to finality of [%s]. NOT VALID", txID)
					}
					return errors.Errorf("transaction [%s] is not valid", txID)
				}
			}
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Is [%s] final? not available yet, wait [err:%s, vc:%d]", txID, err, vd)
			}
		}
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout", txID)
	}
	return errors.Errorf("failed to listen to transaction [%s] for timeout", txID)
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
