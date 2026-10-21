/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

var orderedTransactions = metrics.CounterOpts{
	Subsystem:  "ordering",
	Name:       "ordered_transactions",
	Help:       "The number of ordered transactions.",
	LabelNames: []string{"network"},
}

// Metrics holds the metrics of a Fabric network service.
type Metrics struct {
	OrderedTransactions metrics.Counter
	// ConnState receives the state of the connections to the orderers; nil
	// reports nothing.
	ConnState *connstate.Metrics
}

// NewMetrics registers the network's metrics with p. ConnState is left nil.
func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		OrderedTransactions: p.NewCounter(orderedTransactions),
	}
}
