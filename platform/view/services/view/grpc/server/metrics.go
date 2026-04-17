/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

var (
	requestsReceived = metrics.CounterOpts{
		Name:       "requests_received",
		Help:       "The number of view requests that have been received.",
		LabelNames: []string{"command"},
	}
	requestsCompleted = metrics.CounterOpts{
		Name:       "requests_completed",
		Help:       "The number of view requests that have been completed.",
		LabelNames: []string{"command", "success"},
	}
)

// Metrics models the metrics for the view service server.
type Metrics struct {
	// RequestsReceived returns the number of view requests that have been received.
	RequestsReceived metrics.Counter
	// RequestsCompleted returns the number of view requests that have been completed.
	RequestsCompleted metrics.Counter
}

// NewMetrics returns a new instance of the metrics for the view service server.
func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		RequestsReceived:  p.NewCounter(requestsReceived),
		RequestsCompleted: p.NewCounter(requestsCompleted),
	}
}
