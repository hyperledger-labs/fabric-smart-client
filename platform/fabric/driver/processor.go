/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

type RWSExtractor interface {
	Extract(tx []byte) (ProcessTransaction, RWSet, error)
}

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
	Process(req Request, tx ProcessTransaction, rws RWSet, ns string) error
}

type ProcessorManager interface {
	AddProcessor(ns string, processor Processor) error
	SetDefaultProcessor(processor Processor) error
	AddChannelProcessor(channel string, ns string, processor Processor) error
	ProcessByID(channel, txid string) error
}
