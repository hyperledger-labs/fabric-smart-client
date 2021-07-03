/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

const (
	ConfigTXPrefix = "configtx_"
)

var logger = flogging.MustGetLogger("fabric-sdk.committer")

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

	quietNotifier bool

	listeners map[string][]chan TxEvent
	mutex     sync.Mutex
}

func New(channel string, network Network, finality Finality, waitForEventTimeout time.Duration, quiet bool) (*committer, error) {
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
	}
	return d, nil
}

// Commit commits the transaction in the passed block
func (c *committer) Commit(filteredBlock *pb.FilteredBlock) {
	ledger, err := c.network.Ledger(c.channel)
	if err != nil {
		logger.Panicf("cannot get ledger [%s]", err)
	}
	block, err := ledger.GetBlockByNumber(filteredBlock.Number)
	if err != nil {
		logger.Panicf("cannot get filteredBlock [%s]", err)
	}

	filteredTransactions := filteredBlock.FilteredTransactions
	for i, tx := range filteredTransactions {
		logger.Debugf("commit transaction [%s] in filteredBlock [%d]", tx.Txid, filteredBlock.Number)

		event := &TxEvent{
			Committed:      false,
			DependantTxIDs: []string{},
		}

		switch tx.Type {
		case common.HeaderType_CONFIG:
			c.handleConfig(block, filteredBlock, filteredTransactions, i, event)
		case common.HeaderType_ENDORSER_TRANSACTION:
			c.handleEndorserTransaction(block, filteredBlock, filteredTransactions, i, event)
		}

		c.notify(*event)
	}
}

// IsFinal takes in input a transaction id and waits for its confirmation.
func (c *committer) IsFinal(txid string) error {
	logger.Debugf("Is [%s] final?", txid)

	committer, err := c.network.Committer(c.channel)
	if err != nil {
		return err
	}

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
			return c.finality.IsFinal(txid, c.peerConnectionConfig.Address)
		default:
			panic(fmt.Sprintf("invalid status code, got %c", vd))
		}
	} else {
		logger.Debugf("Is [%s] final? Failed getting transaction status from vault", txid)
		return errors.WithMessagef(err, "failed getting transaction status from vault [%s]", txid)
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
	logger.Debugf("Listen to finality of [%s]", txid)

	ch := make(chan TxEvent, 100)
	c.addListener(txid, ch)
	defer c.deleteListener(txid, ch)

	select {
	case event := <-ch:
		logger.Debugf("Got an answer to finality of [%s]: [%s]", txid, event.Err)
		return event.Err
	case <-time.After(timeout):
		logger.Debugf("Got a timeout for finality of [%s], check the status", txid)
		committer, err := c.network.Committer(c.channel)
		if err != nil {
			return err
		}
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
		logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout, err [%s, %c]", txid, err, vd)
		return errors.Errorf("failed to listen to transaction [%s] for timeout, err [%s, %c]", txid, err, vd)
	}
}
