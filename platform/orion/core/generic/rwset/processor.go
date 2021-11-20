/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.rwset")

type Network interface {
	TransactionManager() driver.TransactionManager
	Name() string
	EnvelopeService() driver.EnvelopeService
}

type RWSExtractor interface {
	Extract(tx []byte) (driver.ProcessTransaction, driver.RWSet, error)
}

type request struct {
	id string
}

func (r *request) ID() string {
	return r.id
}

type processorManager struct {
	sp                view2.ServiceProvider
	network           Network
	defaultProcessor  driver.Processor
	processors        map[string]driver.Processor
	channelProcessors map[string]map[string]driver.Processor
}

func NewProcessorManager(sp view2.ServiceProvider, network Network, defaultProcessor driver.Processor) *processorManager {
	return &processorManager{
		sp:                sp,
		network:           network,
		defaultProcessor:  defaultProcessor,
		processors:        map[string]driver.Processor{},
		channelProcessors: map[string]map[string]driver.Processor{},
	}
}

func (r *processorManager) ProcessByID(channel, txid string) error {
	logger.Debugf("process transaction [%s,%s]", channel, txid)

	req := &request{id: txid}
	logger.Debugf("load transaction content [%s,%s]", channel, txid)

	var rws driver.RWSet
	var tx driver.ProcessTransaction
	var err error
	switch {
	case r.network.EnvelopeService().Exists(txid):
		rws, tx, err = r.getTxFromEvn(txid)
	// case r.network.TransactionService().Exists(txid):
	// 	rws, tx, err = r.getTxFromETx(txid)
	default:
		logger.Debugf("no entry found for [%s,%s]", channel, txid)
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "failed extraction for [%s,%s]", channel, txid)
	}
	defer rws.Done()

	logger.Debugf("process transaction namespaces [%s,%s,%d]", channel, txid, len(rws.Namespaces()))
	for _, ns := range rws.Namespaces() {
		logger.Debugf("process transaction namespace [%s,%s,%s]", channel, txid, ns)

		// TODO: search channel first
		p, ok := r.processors[ns]
		if ok {
			logger.Debugf("process transaction namespace, using custom processor [%s,%s,%s]", channel, txid, ns)
			if err := p.Process(req, tx, rws, ns); err != nil {
				return err
			}
		} else {
			logger.Debugf("process transaction namespace, resorting to default processor [%s,%s,%s]", channel, txid, ns)
			if r.defaultProcessor != nil {
				if err := r.defaultProcessor.Process(req, tx, rws, ns); err != nil {
					return err
				}
			}
			logger.Debugf("no processors found for namespace [%s,%s,%s]", channel, txid, ns)
		}
	}
	return nil
}

func (r *processorManager) AddProcessor(ns string, processor driver.Processor) error {
	r.processors[ns] = processor
	return nil
}

func (r *processorManager) SetDefaultProcessor(processor driver.Processor) error {
	r.defaultProcessor = processor
	return nil
}

func (r *processorManager) getTxFromEvn(txid string) (driver.RWSet, driver.ProcessTransaction, error) {
	// rawEnv, err := ch.EnvelopeService().LoadEnvelope(txid)
	// if err != nil {
	// 	return nil, nil, errors.Wrapf(err, "cannot load envelope [%s]", txid)
	// }
	// logger.Debugf("unmarshal envelope [%s,%s]", ch.Name(), txid)
	// env := &common.Envelope{}
	// err = proto.Unmarshal(rawEnv, env)
	// if err != nil {
	// 	return nil, nil, errors.Wrapf(err, "failed unmarshalling envelope [%s]", txid)
	// }
	// logger.Debugf("unpack envelope [%s,%s]", ch.Name(), txid)
	// upe, err := UnpackEnvelope(r.network.Name(), env)
	// if err != nil {
	// 	return nil, nil, errors.Wrapf(err, "failed unpacking envelope [%s]", txid)
	// }
	// logger.Debugf("retrieve rws [%s,%s]", ch.Name(), txid)
	//
	// rws, err := ch.GetRWSet(txid, upe.Results)
	// if err != nil {
	// 	return nil, nil, err
	// }
	//
	// return rws, upe, nil
	panic("not implemented")
}

func (r *processorManager) getTxFromETx(txid string) (driver.RWSet, driver.ProcessTransaction, error) {
	// raw, err := ch.TransactionService().LoadTransaction(txid)
	// if err != nil {
	// 	return nil, nil, errors.Wrapf(err, "cannot load etx [%s]", txid)
	// }
	// tx, err := r.network.TransactionManager().NewTransactionFromBytes(ch.Name(), raw)
	// if err != nil {
	// 	return nil, nil, err
	// }
	// rws, err := tx.GetRWSet()
	// if err != nil {
	// 	return nil, nil, err
	// }
	//
	// return rws, tx, nil
	panic("not implemented")
}
