/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

type ProcessTransaction interface {
	Network() string
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
	pm driver.ProcessorManager
}

func (pm *ProcessorManager) AddProcessor(ns string, p Processor) error {
	return pm.pm.AddProcessor(ns, &processor{p: p})
}

func (pm *ProcessorManager) SetDefaultProcessor(p Processor) error {
	return pm.pm.SetDefaultProcessor(&processor{p: p})
}

type processor struct {
	p Processor
}

func (p *processor) Process(req driver.Request, tx driver.ProcessTransaction, r driver.RWSet, ns string) error {
	return p.p.Process(
		&request{Request: req},
		&transaction{ProcessTransaction: tx},
		&RWSet{rws: r},
		ns,
	)
}

type request struct {
	driver.Request
}

type transaction struct {
	driver.ProcessTransaction
}
