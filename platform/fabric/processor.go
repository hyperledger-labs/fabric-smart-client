/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
)

type ProcessTransaction interface {
	Network() string
	Channel() string
	ID() string
	FunctionAndParameters() (string, []string)
}

type Request interface {
	ID() string
}

type Processor interface {
	Process(req Request, tx ProcessTransaction, rws *RWSet, ns string) error
}

type ProcessorManager struct {
	pm api.ProcessorManager
}

func (pm *ProcessorManager) AddProcessor(ns string, p Processor) error {
	return pm.pm.AddProcessor(ns, &processor{p: p})
}

func (pm *ProcessorManager) SetDefaultProcessor(p Processor) error {
	return pm.pm.SetDefaultProcessor(&processor{p: p})
}

func (pm *ProcessorManager) AddChannelProcessor(channel, ns string, p Processor) error {
	return pm.pm.AddChannelProcessor(channel, ns, &processor{p: p})
}

type processor struct {
	p Processor
}

func (p *processor) Process(req api.Request, tx api.ProcessTransaction, r api.RWSet, ns string) error {
	return p.p.Process(
		&request{Request: req},
		&transaction{ProcessTransaction: tx},
		&RWSet{rws: r},
		ns,
	)
}

type request struct {
	api.Request
}

type transaction struct {
	api.ProcessTransaction
}
