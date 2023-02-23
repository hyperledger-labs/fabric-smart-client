/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

var (
	requestsReceived = metrics.CounterOpts{
		Namespace:    "view",
		Name:         "requests_received",
		Help:         "The number of view requests that have been received.",
		LabelNames:   []string{"command"},
		StatsdFormat: "%{#fqname}.%{command}.",
	}
	requestsCompleted = metrics.CounterOpts{
		Namespace:    "view",
		Name:         "requests_completed",
		Help:         "The number of view requests that have been completed.",
		LabelNames:   []string{"command", "success"},
		StatsdFormat: "%{#fqname}.%{command}.%{success}",
	}
)

type Metrics struct {
	RequestsReceived  metrics.Counter
	RequestsCompleted metrics.Counter
}

func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		RequestsReceived:  p.NewCounter(requestsReceived),
		RequestsCompleted: p.NewCounter(requestsCompleted),
	}
}
