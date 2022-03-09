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
}

type committer struct {
	vault               Vault
	finality            Finality
	waitForEventTimeout time.Duration

	quietNotifier bool

	listeners      map[string][]chan TxEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration
}

func New(vault Vault, finality Finality, waitForEventTimeout time.Duration, quiet bool) (*committer, error) {
	d := &committer{
		vault:               vault,
		waitForEventTimeout: waitForEventTimeout,
		quietNotifier:       quiet,
		listeners:           map[string][]chan TxEvent{},
		mutex:               sync.Mutex{},
		finality:            finality,
		pollingTimeout:      100 * time.Millisecond,
	}
	return d, nil
}

// Commit commits the transactions in the block passed as argument
func (c *committer) Commit(block *types.AugmentedBlockHeader) error {
	for _, txID := range block.TxIds {
		var event TxEvent
		event.Txid = txID

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
				//if iter >= 2 {
				//	if logger.IsEnabledFor(zapcore.DebugLevel) {
				//		logger.Debugf("Tx [%s] is unknown with no deps, remote check [%d][%s]", txid, iter, debug.Stack())
				//	}
				//	err := c.finality.IsFinal(txid, c.network.PickPeer().Address)
				//	if err == nil {
				//		return nil
				//	}
				//
				//	if vd, _, err2 := c.vault.Status(txid); err2 == nil && vd == driver.Unknown {
				//		return err
				//	}
				//	continue
				//}
				//if logger.IsEnabledFor(zapcore.DebugLevel) {
				//	logger.Debugf("Tx [%s] is unknown with no deps, wait a bit and retry [%d]", txid, iter)
				//}
				//time.Sleep(100 * time.Millisecond)
				// TODO:
				return errors.Errorf("transaction [%s] is unknown", txid)
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
