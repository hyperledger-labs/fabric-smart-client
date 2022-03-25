/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("orion-sdk.rwset")

type Network interface {
	Name() string
	TransactionManager() driver.TransactionManager
	TransactionService() driver.TransactionService
	EnvelopeService() driver.EnvelopeService
	Vault() driver.Vault
}

type request struct {
	id string
}

func (r *request) ID() string {
	return r.id
}

type processorManager struct {
	sp               view2.ServiceProvider
	network          Network
	defaultProcessor driver.Processor
	processors       map[string]driver.Processor
}

func NewProcessorManager(sp view2.ServiceProvider, network Network, defaultProcessor driver.Processor) *processorManager {
	return &processorManager{
		sp:               sp,
		network:          network,
		defaultProcessor: defaultProcessor,
		processors:       map[string]driver.Processor{},
	}
}

func (r *processorManager) ProcessByID(txid string) error {
	logger.Debugf("process transaction [%s]", txid)

	req := &request{id: txid}
	logger.Debugf("load transaction content [%s]", txid)

	var rws driver.RWSet
	var tx driver.ProcessTransaction
	var err error
	switch {
	case r.network.EnvelopeService().Exists(txid):
		rws, tx, err = r.getTxFromEvn(txid)
		if err != nil {
			return errors.Wrapf(err, "failed extraction from envelope [%s]", txid)
		}
	case r.network.TransactionService().Exists(txid):
		rws, tx, err = r.getTxFromETx(txid)
		if err != nil {
			return errors.Wrapf(err, "failed extraction from transaction [%s]", txid)
		}
	default:
		logger.Debugf("no entry found for [%s]", txid)
		return nil
	}
	defer rws.Done()

	logger.Debugf("process transaction namespaces [%s,%d]", txid, len(rws.Namespaces()))
	for _, ns := range rws.Namespaces() {
		logger.Debugf("process transaction namespace [%s,%s]", txid, ns)

		p, ok := r.processors[ns]
		if ok {
			logger.Debugf("process transaction namespace, using custom processor [%s,%s]", txid, ns)
			if err := p.Process(req, tx, rws, ns); err != nil {
				return err
			}
		} else {
			logger.Debugf("process transaction namespace, resorting to default processor [%s,%s]", txid, ns)
			if r.defaultProcessor != nil {
				if err := r.defaultProcessor.Process(req, tx, rws, ns); err != nil {
					return err
				}
			}
			logger.Debugf("no processors found for namespace [%s,%s]", txid, ns)
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
	rawEnv, err := r.network.EnvelopeService().LoadEnvelope(txid)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot load envelope [%s]", txid)
	}
	logger.Debugf("unmarshal envelope [%s]", txid)
	env := r.network.TransactionManager().NewEnvelope()
	if err := env.FromBytes(rawEnv); err != nil {
		return nil, nil, errors.Wrapf(err, "cannot unmarshal envelope [%s]", txid)
	}
	rws, err := r.network.Vault().GetRWSet(txid, env.Results())
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot unmarshal envelope [%s]", txid)
	}
	return rws, &ProcessTransaction{
		network: r.network.Name(),
		txID:    txid,
	}, nil
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

type ProcessTransaction struct {
	network string
	txID    string
}

func (p *ProcessTransaction) Network() string {
	return p.network
}

func (p *ProcessTransaction) ID() string {
	return p.txID
}

func (p *ProcessTransaction) FunctionAndParameters() (string, []string) {
	//TODO implement me
	panic("implement me")
}
