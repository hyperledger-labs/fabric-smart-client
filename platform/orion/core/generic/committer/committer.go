/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"fmt"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
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
	vault               Vault
	finality            Finality
	pm                  ProcessorManager
	waitForEventTimeout time.Duration

	quietNotifier bool

	listeners      map[string][]chan TxEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration
}

func New(pm ProcessorManager, vault Vault, finality Finality, waitForEventTimeout time.Duration, quiet bool) (*committer, error) {
	d := &committer{
		vault:               vault,
		waitForEventTimeout: waitForEventTimeout,
		quietNotifier:       quiet,
		listeners:           map[string][]chan TxEvent{},
		mutex:               sync.Mutex{},
		finality:            finality,
		pm:                  pm,
		pollingTimeout:      100 * time.Millisecond,
	}
	return d, nil
}

// Commit commits the transactions in the block passed as argument
func (c *committer) Commit(block *types.AugmentedBlockHeader) error {
	bn := block.Header.BaseHeader.Number
	for i, txID := range block.TxIds {
		var event TxEvent
		event.Txid = txID

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
		default:
			// rollback
			if err := c.vault.DiscardTx(txID); err != nil {
				return errors.WithMessagef(err, "failed to discard tx %s", txID)
			}
		}
		// parse transactions
		c.notify(event)
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
	return c.listenTo(txid, c.waitForEventTimeout)
}

func (c *committer) addListener(txid string, ch chan TxEvent) {
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

func (c *committer) deleteListener(txid string, ch chan TxEvent) {
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

func (c *committer) notify(event TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

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

func (c *committer) listenTo(txid string, timeout time.Duration) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Listen to finality of [%s]", txid)
	}

	// notice that adding the listener can happen after the event we are looking for has already happened
	// therefore we need to check more often before the timeout happens
	ch := make(chan TxEvent, 100)
	c.addListener(txid, ch)
	defer c.deleteListener(txid, ch)

	iterations := int(timeout.Milliseconds() / c.pollingTimeout.Milliseconds())
	if iterations == 0 {
		iterations = 1
	}
	for i := 0; i < iterations; i++ {
		select {
		case event := <-ch:
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Got an answer to finality of [%s]: [%s]", txid, event.Err)
			}
			return event.Err
		case <-time.After(c.pollingTimeout):
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Got a timeout for finality of [%s], check the status", txid)
			}
			vd, err := c.vault.Status(txid)
			if err == nil {
				switch vd {
				case driver.Valid:
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("Listen to finality of [%s]. VALID", txid)
					}
					return nil
				case driver.Invalid:
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("Listen to finality of [%s]. NOT VALID", txid)
					}
					return errors.Errorf("transaction [%s] is not valid", txid)
				}
			}
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Is [%s] final? not available yet, wait [err:%s, vc:%d]", txid, err, vd)
			}
		}
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout", txid)
	}
	return errors.Errorf("failed to listen to transaction [%s] for timeout", txid)
}
