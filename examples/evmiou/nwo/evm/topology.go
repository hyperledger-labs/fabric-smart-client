/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evm

import "time"

// TopologyName is both the name and the type of the evm topology.
const TopologyName = "evm"

const (
	defaultGasMultiplier = 1.2
	defaultPollInterval  = time.Second
	defaultTimeout       = time.Minute
)

// Topology configures the EVM network GenerateArtifacts brings up: which FSC nodes act as
// contract approvers and which submit transactions (and so need funding), the gas multiplier
// Submitter applies, and the poll interval/timeout WaitReceipt uses.
type Topology struct {
	TopologyName string `yaml:"name,omitempty"`
	TopologyType string `yaml:"type,omitempty"`

	// Approvers lists the FSC node names whose generated EVM key becomes an approver address the
	// IOU contract is deployed with.
	Approvers []string `yaml:"approvers,omitempty"`
	// Submitters lists the FSC node names GenerateArtifacts funds with 10 ETH so they can submit
	// transactions.
	Submitters []string `yaml:"submitters,omitempty"`

	// GasMultiplier is applied to EstimateGas's result by every node's chain.Submitter.
	GasMultiplier float64 `yaml:"gasMultiplier,omitempty"`
	// PollInterval and Timeout bound how long a node's chain.WaitReceipt waits for a receipt.
	PollInterval time.Duration `yaml:"pollInterval,omitempty"`
	Timeout      time.Duration `yaml:"timeout,omitempty"`
}

// NewTopology returns a Topology with a gas multiplier of 1.2, a poll interval of 1s and a
// timeout of 1m.
func NewTopology() *Topology {
	return &Topology{
		TopologyName:  TopologyName,
		TopologyType:  TopologyName,
		GasMultiplier: defaultGasMultiplier,
		PollInterval:  defaultPollInterval,
		Timeout:       defaultTimeout,
	}
}

// Name returns the topology's name.
func (t *Topology) Name() string { return t.TopologyName }

// Type returns the topology's type, which nwo uses to find this package's PlatformFactory.
func (t *Topology) Type() string { return t.TopologyType }
