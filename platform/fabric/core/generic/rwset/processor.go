/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger("fabric-sdk.rwset")

type ChannelProvider interface {
	Channel(name string) (driver.Channel, error)
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
	channelProvider   ChannelProvider
	defaultProcessor  driver.Processor
	processors        map[string]driver.Processor
	channelProcessors map[string]map[string]driver.Processor
}

func NewProcessorManager(
	channelProvider ChannelProvider,
	defaultProcessor driver.Processor,
) *processorManager {
	return &processorManager{
		channelProvider:   channelProvider,
		defaultProcessor:  defaultProcessor,
		processors:        map[string]driver.Processor{},
		channelProcessors: map[string]map[string]driver.Processor{},
	}
}

func (r *processorManager) ProcessByID(channel, txID string) error {
	logger.Debugf("process transaction [%s,%s]", channel, txID)

	ch, err := r.channelProvider.Channel(channel)
	if err != nil {
		return errors.Wrapf(err, "failed getting channel [%s]", ch)
	}

	req := &request{id: txID}
	logger.Debugf("load transaction content [%s,%s]", channel, txID)

	var rws driver.RWSet
	var tx driver.ProcessTransaction
	switch {
	case ch.EnvelopeService().Exists(txID):
		rws, tx, err = ch.RWSetLoader().GetRWSetFromEvn(txID)
	case ch.TransactionService().Exists(txID):
		rws, tx, err = ch.RWSetLoader().GetRWSetFromETx(txID)
	default:
		logger.Debugf("no entry found for [%s,%s]", channel, txID)
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "failed extraction for [%s,%s]", channel, txID)
	}
	defer rws.Done()

	logger.Debugf("process transaction namespaces [%s,%s,%d]", channel, txID, len(rws.Namespaces()))
	for _, ns := range rws.Namespaces() {
		logger.Debugf("process transaction namespace [%s,%s,%s]", channel, txID, ns)

		// TODO: search channel first
		p, ok := r.processors[ns]
		if ok {
			logger.Debugf("process transaction namespace, using custom processor [%s,%s,%s]", channel, txID, ns)
			if err := p.Process(req, tx, rws, ns); err != nil {
				return err
			}
		} else {
			logger.Debugf("process transaction namespace, resorting to default processor [%s,%s,%s]", channel, txID, ns)
			if r.defaultProcessor != nil {
				if err := r.defaultProcessor.Process(req, tx, rws, ns); err != nil {
					return err
				}
			}
			logger.Debugf("no processors found for namespace [%s,%s,%s]", channel, txID, ns)
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

func (r *processorManager) AddChannelProcessor(channel string, ns string, processor driver.Processor) error {
	r.channelProcessors[channel][ns] = processor
	return nil
}
