/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hyperledger/fabric/protoutil"

	"go.uber.org/zap/zapcore"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

const (
	ConfigTXPrefix = "configtx_"
)

var logger = flogging.MustGetLogger("fabric-sdk.committer")

type Metrics interface {
	EmitKey(val float32, event ...string)
}

type Finality interface {
	IsFinal(txID string, address string) error
}

type Network interface {
	Committer(channel string) (driver.Committer, error)
	PickPeer() *grpc.ConnectionConfig
	Ledger(channel string) (driver.Ledger, error)
}

type committer struct {
	channel             string
	network             Network
	finality            Finality
	waitForEventTimeout time.Duration
	metrics             Metrics

	quietNotifier bool

	listeners       map[string][]chan TxEvent
	mutex           sync.Mutex
	pollingTimeout  time.Duration
	serviceProvider view.ServiceProvider
}

func New(channel string, network Network, finality Finality, waitForEventTimeout time.Duration, quiet bool, metrics Metrics, sp view.ServiceProvider) (*committer, error) {
	if len(channel) == 0 {
		panic("expected a channel, got empty string")
	}

	d := &committer{
		channel:             channel,
		network:             network,
		waitForEventTimeout: waitForEventTimeout,
		quietNotifier:       quiet,
		listeners:           map[string][]chan TxEvent{},
		mutex:               sync.Mutex{},
		finality:            finality,
		pollingTimeout:      100 * time.Millisecond,
		metrics:             metrics,
		serviceProvider:     sp,
	}
	return d, nil
}

// Commit commits the transactions in the block passed as argument
func (c *committer) Commit(block *common.Block) error {

	for i, tx := range block.Data.Data {
		env, err := protoutil.UnmarshalEnvelope(tx)

		if err != nil {
			logger.Errorf("Error getting tx from block: %s", err)
			return err
		}
		payl, err := protoutil.UnmarshalPayload(env.Payload)
		if err != nil {
			logger.Errorf("[%s] unmarshal payload failed: %s", c.channel, err)
			return err
		}
		tx, err := protoutil.UnmarshalTransaction(payl.Data)
		if err != nil {
			logger.Errorf("[%s] unmarshal tranwaction failed: %s", c.channel, err)
			return err
		}
		chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
		if err != nil {
			logger.Errorf("[%s] unmarshal channel header failed: %s", c.channel, err)
			return err
		}

		var event TxEvent

		c.metrics.EmitKey(0, "committer", "start", "Commit", chdr.TxId)
		switch common.HeaderType(chdr.Type) {
		case common.HeaderType_CONFIG:
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("[%s] Config transaction received: %s", c.channel, chdr.TxId)
			}
			c.handleConfig(block, i, env)
		case common.HeaderType_ENDORSER_TRANSACTION:
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("[%s] Endorser transaction received: %s", c.channel, chdr.TxId)
			}
			if len(block.Metadata.Metadata) < int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
				return errors.Errorf("block metadata lacks transaction filter")
			}
			c.handleEndorserTransaction(block, i, &event, env, chdr, tx)
		default:
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("[%s] Received unhandled transaction type: %s", c.channel, chdr.Type)
			}
		}
		c.metrics.EmitKey(0, "committer", "end", "Commit", chdr.TxId)

		c.notify(event)

		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("commit transaction [%s] in filteredBlock [%d]", chdr.TxId, block.Header.Number)
		}
	}
	return nil
}

// IsFinal takes in input a transaction id and waits for its confirmation.
func (c *committer) IsFinal(txid string) error {
	c.metrics.EmitKey(0, "committer", "start", "IsFinal", txid)
	defer c.metrics.EmitKey(0, "committer", "end", "IsFinal", txid)

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final?", txid)
	}

	committer, err := c.network.Committer(c.channel)
	if err != nil {
		return err
	}

	for iter := 0; iter < 3; iter++ {
		vd, deps, err := committer.Status(txid)
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
					logger.Debugf("Tx [%s] is known with deps [%v]", txid, deps)
				}
				if len(deps) != 0 {
					for _, id := range deps {
						if logger.IsEnabledFor(zapcore.DebugLevel) {
							logger.Debugf("Check finality of dependant transaction [%s]", id)
						}
						err := c.IsFinal(id)
						if err != nil {
							logger.Errorf("Check finality of dependant transaction [%s], failed [%s]", id, err)
							return err
						}
					}
					return nil
				}
			case driver.HasDependencies:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is unknown with deps [%v]", txid, deps)
				}
				if len(deps) != 0 {
					for _, id := range deps {
						if logger.IsEnabledFor(zapcore.DebugLevel) {
							logger.Debugf("Check finality of dependant transaction [%s]", id)
						}
						err := c.IsFinal(id)
						if err != nil {
							logger.Errorf("Check finality of dependant transaction [%s], failed [%s]", id, err)
							return err
						}
					}
					return nil
				}
				return c.finality.IsFinal(txid, c.network.PickPeer().Address)
			case driver.Unknown:
				if iter >= 2 {
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("Tx [%s] is unknown with no deps, remote check [%d][%s]", txid, iter, debug.Stack())
					}
					err := c.finality.IsFinal(txid, c.network.PickPeer().Address)
					if err == nil {
						return nil
					}

					if vd, _, err2 := committer.Status(txid); err2 == nil && vd == driver.Unknown {
						return err
					}
					continue
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

func (c *committer) notifyChaincodeListeners(event *ChaincodeEvent) error {
	publisher, err := events.GetPublisher(c.serviceProvider)
	if err != nil {
		return errors.Wrap(err, "failed to get event publisher")
	}
	publisher.Publish(event)
	fmt.Println("done notifyChaincodeListeners")
	return nil
}

func (c *committer) listenTo(txid string, timeout time.Duration) error {
	c.metrics.EmitKey(0, "committer", "start", "listenTo", txid)
	defer c.metrics.EmitKey(0, "committer", "end", "listenTo", txid)

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Listen to finality of [%s]", txid)
	}

	// notice that adding the listener can happen after the event we are looking for has already happened
	// therefore we need to check more often before the timeout happens
	ch := make(chan TxEvent, 100)
	c.addListener(txid, ch)
	defer c.deleteListener(txid, ch)

	committer, err := c.network.Committer(c.channel)
	if err != nil {
		return err
	}
	iterations := int(timeout.Milliseconds() / c.pollingTimeout.Milliseconds())
	if iterations == 0 {
		iterations = 1
	}
	for i := 0; i < iterations; i++ {
		timeout := time.NewTimer(c.pollingTimeout)

		select {
		case event := <-ch:
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Got an answer to finality of [%s]: [%s]", txid, event.Err)
			}
			timeout.Stop()
			return event.Err
		case <-timeout.C:
			timeout.Stop()
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Got a timeout for finality of [%s], check the status", txid)
			}
			vd, _, err := committer.Status(txid)
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
