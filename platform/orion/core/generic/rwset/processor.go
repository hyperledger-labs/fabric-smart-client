/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"context"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger("orion-sdk.rwset")

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
	network          Network
	defaultProcessor driver.Processor
	processors       map[string]driver.Processor
}

func NewProcessorManager(network Network, defaultProcessor driver.Processor) *processorManager {
	return &processorManager{
		network:          network,
		defaultProcessor: defaultProcessor,
		processors:       map[string]driver.Processor{},
	}
}

func (r *processorManager) ProcessByID(ctx context.Context, txID driver2.TxID) error {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_process_by_id")
	defer span.AddEvent("end_process_by_id")

	logger.Debugf("process transaction [%s]", txID)

	req := &request{id: txID}
	logger.Debugf("load transaction content [%s]", txID)

	var rws driver.RWSet
	var tx driver.ProcessTransaction
	var err error
	switch {
	case r.network.EnvelopeService().Exists(txID):
		rws, tx, err = r.getTxFromEvn(txID)
		if err != nil {
			return errors.Wrapf(err, "failed extraction from envelope [%s]", txID)
		}
	case r.network.TransactionService().Exists(txID):
		rws, tx, err = r.getTxFromETx(txID)
		if err != nil {
			return errors.Wrapf(err, "failed extraction from transaction [%s]", txID)
		}
	default:
		logger.Debugf("no entry found for [%s]", txID)
		return nil
	}
	defer rws.Done()

	logger.Debugf("process transaction namespaces [%s,%d]", txID, len(rws.Namespaces()))
	for _, ns := range rws.Namespaces() {
		logger.Debugf("process transaction namespace [%s,%s]", txID, ns)

		p, ok := r.processors[ns]
		if ok {
			logger.Debugf("process transaction namespace, using custom processor [%s,%s]", txID, ns)
			if err := p.Process(req, tx, rws, ns); err != nil {
				return err
			}
		} else {
			logger.Debugf("process transaction namespace, resorting to default processor [%s,%s]", txID, ns)
			if r.defaultProcessor != nil {
				if err := r.defaultProcessor.Process(req, tx, rws, ns); err != nil {
					return err
				}
			}
			logger.Debugf("no processors found for namespace [%s,%s]", txID, ns)
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
	rws, err := r.network.Vault().NewRWSetFromBytes(context.Background(), txid, env.Results())
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
	// TODO implement me
	panic("implement me")
}
