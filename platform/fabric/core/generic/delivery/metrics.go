/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// failureClassLabel labels CommitFailures with the failureClass that stopped
// delivery, so that a stalled channel can be told apart from a retried one
// without reading the logs.
const failureClassLabel = "class"

// Metrics counts the commit failures a Delivery acts on.
//
// A channel whose delivery has stopped produces no further blocks and no further
// errors, so its failure is invisible in every other signal the node emits: the
// block-commit histogram simply stops receiving observations, which is
// indistinguishable from an idle channel. These counters exist so that the stop
// itself is recorded, and are the signal an alert should be built on.
type Metrics struct {
	// CommitRetries counts individual retried commit attempts. A rising count
	// with no CommitFailures is a channel that is recovering on its own.
	CommitRetries metrics.Counter
	// CommitFailures counts the commit failures that stopped a delivery,
	// labelled by failure class. Any increment means a channel has stopped
	// making progress and will not resume without intervention.
	CommitFailures metrics.Counter
}

// NewMetrics builds the delivery counters from p. A nil p yields counters that
// discard their observations, so that a Delivery built without a metrics provider
// still runs; the counters are a diagnostic, never a precondition for delivering
// blocks.
func NewMetrics(p metrics.Provider) *Metrics {
	if p == nil {
		p = &disabled.Provider{}
	}
	return &Metrics{
		// The namespace and subsystem are left to the provider, which derives
		// them from this package's path, as every other Fabric metric family
		// does.
		CommitRetries: p.NewCounter(metrics.CounterOpts{
			Name: "commit_retries",
			Help: "Number of block commit attempts retried after a transient failure",
		}),
		CommitFailures: p.NewCounter(metrics.CounterOpts{
			Name:       "commit_failures",
			Help:       "Number of block commit failures that stopped the delivery service, by failure class",
			LabelNames: []string{failureClassLabel},
		}),
	}
}
