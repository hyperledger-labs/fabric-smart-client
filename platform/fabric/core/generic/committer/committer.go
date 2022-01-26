/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/fabric/protoutil"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

const (
	ConfigTXPrefix = "configtx_"
)

var logger = flogging.MustGetLogger("fabric-sdk.committer")

var (
	ErrQSCCUnreachable = errors.New("error querying QSCC")
)

type Metrics interface {
	EmitKey(val float32, event ...string)
}

type Finality interface {
	IsFinal(txID string, address string) error
}

type Network interface {
	Committer(channel string) (driver.Committer, error)
	Peers() []*grpc.ConnectionConfig
	Ledger(channel string) (driver.Ledger, error)
}

type committer struct {
	channel              string
	network              Network
	finality             Finality
	peerConnectionConfig *grpc.ConnectionConfig
	waitForEventTimeout  time.Duration
	metrics              Metrics

	quietNotifier bool

	listeners      map[string][]chan TxEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration
}

func New(channel string, network Network, finality Finality, waitForEventTimeout time.Duration, quiet bool, metrics Metrics) (*committer, error) {
	if len(channel) == 0 {
		panic("expected a channel, got empty string")
	}

	d := &committer{
		channel:              channel,
		network:              network,
		waitForEventTimeout:  waitForEventTimeout,
		quietNotifier:        quiet,
		peerConnectionConfig: network.Peers()[0],
		listeners:            map[string][]chan TxEvent{},
		mutex:                sync.Mutex{},
		finality:             finality,
		pollingTimeout:       100 * time.Millisecond,
		metrics:              metrics,
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
		chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
		if err != nil {
			logger.Errorf("[%s] unmarshal channel header failed: %s", c.channel, err)
			return err
		}

		var event TxEvent

		c.metrics.EmitKey(0, "committer", "start", "Commit", chdr.TxId)
		switch common.HeaderType(chdr.Type) {
		case common.HeaderType_CONFIG:
			logger.Debugf("[%s] Config transaction received: %s", c.channel, chdr.TxId)
			c.handleConfig(block, i, env)
		case common.HeaderType_ENDORSER_TRANSACTION:
			logger.Debugf("[%s] Endorser transaction received: %s", c.channel, chdr.TxId)
			if len(block.Metadata.Metadata) < int(common.BlockMetadataIndex_TRANSACTIONS_FILTER) {
				return errors.Errorf("block metadata lacks transaction filter")
			}
			c.handleEndorserTransaction(block, i, &event, env, chdr)
		default:
			logger.Debugf("[%s] Received unhandled transaction type: %s", c.channel, chdr.Type)
		}
		c.metrics.EmitKey(0, "committer", "end", "Commit", chdr.TxId)

		c.notify(event)

		logger.Debugf("commit transaction [%s] in filteredBlock [%d]", chdr.TxId, block.Header.Number)
	}

	return nil
}

// IsFinal takes in input a transaction id and waits for its confirmation.
func (c *committer) IsFinal(txid string) error {
	c.metrics.EmitKey(0, "committer", "start", "IsFinal", txid)
	defer c.metrics.EmitKey(0, "committer", "end", "IsFinal", txid)

	logger.Debugf("Is [%s] final?", txid)

	committer, err := c.network.Committer(c.channel)
	if err != nil {
		return err
	}

	iter := 0
	for {
		vd, deps, err := committer.Status(txid)
		if err == nil {
			switch vd {
			case driver.Valid:
				logger.Debugf("Tx [%s] is valid", txid)
				return nil
			case driver.Invalid:
				logger.Debugf("Tx [%s] is not valid", txid)
				return errors.Errorf("transaction [%s] is not valid", txid)
			case driver.Busy:
				logger.Debugf("Tx [%s] is known with deps [%v]", txid, deps)
				if len(deps) != 0 {
					for _, id := range deps {
						logger.Debugf("Check finality of dependant transaction [%s]", id)
						err := c.IsFinal(id)
						if err != nil {
							logger.Errorf("Check finality of dependant transaction [%s], failed [%s]", id, err)
							return err
						}
					}
					return nil
				}
			case driver.HasDependencies:
				logger.Debugf("Tx [%s] is unknown with deps [%v]", txid, deps)
				if len(deps) != 0 {
					for _, id := range deps {
						logger.Debugf("Check finality of dependant transaction [%s]", id)
						err := c.IsFinal(id)
						if err != nil {
							logger.Errorf("Check finality of dependant transaction [%s], failed [%s]", id, err)
							return err
						}
					}
					return nil
				}
				return c.finality.IsFinal(txid, c.peerConnectionConfig.Address)
			case driver.Unknown:
				if iter == 2 {
					logger.Debugf("Tx [%s] is unknown with no deps", txid)
					return c.finality.IsFinal(txid, c.peerConnectionConfig.Address)
				}
				iter++
				time.Sleep(100 * time.Millisecond)
				continue
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
	logger.Debugf("Notify the finality of [%s] to [%d] listeners, event: [%v]", event.Txid, len(listeners), event)
	for _, listener := range listeners {
		listener <- event
	}

	for _, txid := range event.DependantTxIDs {
		listeners := c.listeners[txid]
		logger.Debugf("Notify the finality of [%s] (dependant) to [%d] listeners, event: [%v]", txid, len(listeners), event)
		for _, listener := range listeners {
			listener <- event
		}
	}
}

func (c *committer) listenTo(txid string, timeout time.Duration) error {
	c.metrics.EmitKey(0, "committer", "start", "listenTo", txid)
	defer c.metrics.EmitKey(0, "committer", "end", "listenTo", txid)

	logger.Debugf("Listen to finality of [%s]", txid)

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
		select {
		case event := <-ch:
			logger.Debugf("Got an answer to finality of [%s]: [%s]", txid, event.Err)
			return event.Err
		case <-time.After(c.pollingTimeout):
			logger.Debugf("Got a timeout for finality of [%s], check the status", txid)
			vd, _, err := committer.Status(txid)
			if err == nil {
				switch vd {
				case driver.Valid:
					logger.Debugf("Listen to finality of [%s]. VALID", txid)
					return nil
				case driver.Invalid:
					logger.Debugf("Listen to finality of [%s]. NOT VALID", txid)
					return errors.Errorf("transaction [%s] is not valid", txid)
				}
			}
			logger.Debugf("Is [%s] final? not available yet, wait [%s, %c]", txid, err, vd)
		}
	}
	logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout", txid)
	return errors.Errorf("failed to listen to transaction [%s] for timeout", txid)
}
