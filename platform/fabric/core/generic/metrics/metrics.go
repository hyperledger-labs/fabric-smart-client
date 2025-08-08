/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

var (
	orderedTransactions = metrics.CounterOpts{
		Subsystem:  "ordering",
		Name:       "ordered_transactions",
		Help:       "The number of ordered transactions.",
		LabelNames: []string{"network"},
	}
)

type Metrics struct {
	OrderedTransactions metrics.Counter
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		OrderedTransactions: p.NewCounter(orderedTransactions),
	}
}
