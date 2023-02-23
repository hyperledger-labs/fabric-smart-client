/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const (
	ConfigTXPrefix = "configtx_"
)

var logger = flogging.MustGetLogger("fabric-sdk.Committer")

type Finality interface {
	IsFinal(txID string, address string) error
}

type Network interface {
	Committer(channel string) (driver.Committer, error)
	Channel(channel string) (driver.Channel, error)
	PickPeer(funcType driver.PeerFunctionType) *grpc.ConnectionConfig
	Ledger(channel string) (driver.Ledger, error)
}

type TransactionHandler = func(block *common.Block, i int, event *TxEvent, env *common.Envelope, chHdr *common.ChannelHeader) error

type Committer struct {
	Channel             string
	Network             Network
	Finality            Finality
	WaitForEventTimeout time.Duration
	Tracer              tracing.Tracer
	Handlers            map[common.HeaderType]TransactionHandler
	QuietNotifier       bool

	listeners      map[string][]chan TxEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration
	publisher      events.Publisher
}

func New(channel string, network Network, finality Finality, waitForEventTimeout time.Duration, quiet bool, metrics tracing.Tracer, publisher events.Publisher) (*Committer, error) {
	if len(channel) == 0 {
		return nil, errors.Errorf("expected a channel, got empty string")
	}

	d := &Committer{
		Channel:             channel,
		Network:             network,
		WaitForEventTimeout: waitForEventTimeout,
		QuietNotifier:       quiet,
		listeners:           map[string][]chan TxEvent{},
		mutex:               sync.Mutex{},
		Finality:            finality,
		pollingTimeout:      100 * time.Millisecond,
		Tracer:              metrics,
		publisher:           publisher,
		Handlers:            map[common.HeaderType]TransactionHandler{},
	}
	d.Handlers[common.HeaderType_CONFIG] = d.HandleConfig
	d.Handlers[common.HeaderType_ENDORSER_TRANSACTION] = d.HandleEndorserTransaction
	return d, nil
}

// Commit commits the transactions in the block passed as argument
func (c *Committer) Commit(block *common.Block) error {
	c.Tracer.StartAt("commit", time.Now())
	for i, tx := range block.Data.Data {

		env, err := protoutil.UnmarshalEnvelope(tx)
		if err != nil {
			logger.Errorf("Error getting tx from block: %s", err)
			return err
		}
		payl, err := protoutil.UnmarshalPayload(env.Payload)
		if err != nil {
			logger.Errorf("[%s] unmarshal payload failed: %s", c.Channel, err)
			return err
		}
		chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
		if err != nil {
			logger.Errorf("[%s] unmarshal channel header failed: %s", c.Channel, err)
			return err
		}

		var event TxEvent
		c.Tracer.AddEventAt("commit", "start", time.Now())
		handler, ok := c.Handlers[common.HeaderType(chdr.Type)]
		if ok {
			if err := handler(block, i, &event, env, chdr); err != nil {
				return err
			}
		} else {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("[%s] Received unhandled transaction type: %s", c.Channel, chdr.Type)
			}
		}
		c.Tracer.AddEventAt("commit", "end", time.Now())

		c.Notify(event)
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("commit transaction [%s] in filteredBlock [%d]", chdr.TxId, block.Header.Number)
		}
	}

	return nil
}

// IsFinal takes in input a transaction id and waits for its confirmation
// with the respect to the passed context that can be used to set a deadline
// for the waiting time.
func (c *Committer) IsFinal(ctx context.Context, txID string) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final?", txID)
	}

	committer, err := c.Network.Committer(c.Channel)
	if err != nil {
		return err
	}

	for iter := 0; iter < 3; iter++ {
		vd, deps, err := committer.Status(txID)
		if err == nil {
			switch vd {
			case driver.Valid:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is valid", txID)
				}
				return nil
			case driver.Invalid:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is not valid", txID)
				}
				return errors.Errorf("transaction [%s] is not valid", txID)
			case driver.Busy:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is known with deps [%v]", txID, deps)
				}
				if len(deps) != 0 {
					for _, id := range deps {
						if logger.IsEnabledFor(zapcore.DebugLevel) {
							logger.Debugf("Check finality of dependant transaction [%s]", id)
						}
						err := c.IsFinal(ctx, id)
						if err != nil {
							logger.Errorf("Check finality of dependant transaction [%s], failed [%s]", id, err)
							return err
						}
					}
					return nil
				}
			case driver.HasDependencies:
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is unknown with deps [%v]", txID, deps)
				}
				if len(deps) != 0 {
					for _, id := range deps {
						if logger.IsEnabledFor(zapcore.DebugLevel) {
							logger.Debugf("Check finality of dependant transaction [%s]", id)
						}
						err := c.IsFinal(ctx, id)
						if err != nil {
							logger.Errorf("Check finality of dependant transaction [%s], failed [%s]", id, err)
							return err
						}
					}
					return nil
				}
				return c.Finality.IsFinal(txID, c.Network.PickPeer(driver.PeerForFinality).Address)
			case driver.Unknown:
				if iter >= 2 {
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("Tx [%s] is unknown with no deps, remote check [%d][%s]", txID, iter, debug.Stack())
					}
					err := c.Finality.IsFinal(txID, c.Network.PickPeer(driver.PeerForFinality).Address)
					if err == nil {
						return nil
					}

					if vd, _, err2 := committer.Status(txID); err2 == nil && vd == driver.Unknown {
						return err
					}
					continue
				}
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("Tx [%s] is unknown with no deps, wait a bit and retry [%d]", txID, iter)
				}
				time.Sleep(100 * time.Millisecond)
			default:
				return errors.Errorf("invalid status code, got [%c]", vd)
			}
		} else {
			logger.Errorf("Is [%s] final? Failed getting transaction status from vault", txID)
			return errors.WithMessagef(err, "failed getting transaction status from vault [%s]", txID)
		}
	}
	// Listen to the event
	return c.listenTo(ctx, txID, c.WaitForEventTimeout)
}

func (c *Committer) addListener(txid string, ch chan TxEvent) {
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

func (c *Committer) deleteListener(txid string, ch chan TxEvent) {
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

func (c *Committer) Notify(event TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if event.Err != nil && !c.QuietNotifier {
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

// notifyChaincodeListeners notifies the chaincode event to the registered chaincode listeners.
func (c *Committer) notifyChaincodeListeners(event *ChaincodeEvent) {
	c.publisher.Publish(event)
}

func (c *Committer) listenTo(ctx context.Context, txid string, timeout time.Duration) error {
	c.Tracer.Start("committer-listenTo-start")

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Listen to finality of [%s]", txid)
	}

	// notice that adding the listener can happen after the event we are looking for has already happened
	// therefore we need to check more often before the timeout happens
	ch := make(chan TxEvent, 100)
	c.addListener(txid, ch)
	defer c.deleteListener(txid, ch)

	committer, err := c.Network.Committer(c.Channel)
	if err != nil {
		return err
	}
	iterations := int(timeout.Milliseconds() / c.pollingTimeout.Milliseconds())
	if iterations == 0 {
		iterations = 1
	}
	for i := 0; i < iterations; i++ {
		timeout := time.NewTimer(c.pollingTimeout)

		stop := false
		select {
		case <-ctx.Done():
			timeout.Stop()
			stop = true
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
		if stop {
			break
		}
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout", txid)
	}
	c.Tracer.End("committer-listenTo-end")
	return errors.Errorf("failed to listen to transaction [%s] for timeout", txid)
}
