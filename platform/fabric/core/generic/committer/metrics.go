/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

const (
	// failureClassLabel labels CommitFailures with the failureClass that ended
	// the commit, so that a channel stopped by a configuration it cannot apply
	// can be told apart from one stopped by a broken invariant.
	failureClassLabel = "class"
	// networkLabel and channelLabel identify which channel a commit counter
	// belongs to. A node serves several channels through one process-wide metrics
	// provider, so without them every channel shares one series.
	networkLabel = "network"
	channelLabel = "channel"
)

type Metrics struct {
	NotifyStatusDuration   metrics.Histogram
	NotifyFinalityDuration metrics.Histogram
	PostFinalityDuration   metrics.Histogram
	HandlerDuration        metrics.Histogram
	EventQueueDuration     metrics.Histogram
	BlockCommitDuration    metrics.Histogram
	EventQueueLength       metrics.Gauge

	// CommitRetries counts individual retried commit attempts. A rising count
	// with no CommitFailures is a channel absorbing transient faults and
	// recovering on its own.
	CommitRetries metrics.Counter
	// CommitFailures counts the commit failures that were returned to the caller
	// and therefore stopped that channel's block stream, labelled by failure
	// class. A channel that has stopped emits no further blocks and no further
	// errors, so it is invisible in every duration metric here — BlockCommitDuration
	// simply stops receiving observations, which looks identical to an idle
	// channel. This counter is what an alert should watch.
	CommitFailures metrics.Counter

	Commits trace.Tracer
	Listens trace.Tracer
}

// NewMetrics builds the committer's metrics. network and channel are bound as
// labels on the commit counters rather than left to the metric name, so that a
// node running several channels reports which one stopped: the metrics provider
// is process-wide, and without them every channel would increment the same
// series and an operator would learn only that something stopped.
func NewMetrics(p tracing.Provider, m metrics.Provider, network, channel string) *Metrics {
	return &Metrics{
		CommitRetries: m.NewCounter(metrics.CounterOpts{
			Name:       "commit_retries",
			Help:       "Number of block commit attempts retried after a transient failure",
			LabelNames: []string{networkLabel, channelLabel},
		}).With(networkLabel, network, channelLabel, channel),
		CommitFailures: m.NewCounter(metrics.CounterOpts{
			Name:       "commit_failures",
			Help:       "Number of block commit failures that stopped the channel's block stream, by failure class",
			LabelNames: []string{networkLabel, channelLabel, failureClassLabel},
		}).With(networkLabel, network, channelLabel, channel),
		NotifyStatusDuration: m.NewHistogram(metrics.HistogramOpts{
			Name:    "notify_status",
			Help:    "Histogram for the duration of notifyStatus",
			Buckets: utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		NotifyFinalityDuration: m.NewHistogram(metrics.HistogramOpts{
			Name:    "notify_finality",
			Help:    "Histogram for the duration of notifyFinality",
			Buckets: utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		PostFinalityDuration: m.NewHistogram(metrics.HistogramOpts{
			Name:    "post_finality",
			Help:    "Histogram for the duration of postFinality",
			Buckets: utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		HandlerDuration: m.NewHistogram(metrics.HistogramOpts{
			Name:       "handler",
			Help:       "Histogram for the duration of the committer handler",
			LabelNames: []string{"status"},
			Buckets:    utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		BlockCommitDuration: m.NewHistogram(metrics.HistogramOpts{
			Name:    "block_commit",
			Help:    "Histogram for the duration of the block commit",
			Buckets: utils.ExponentialBucketTimeRange(0, 5*time.Second, 15),
		}),
		EventQueueDuration: m.NewHistogram(metrics.HistogramOpts{
			Name:    "event_queue",
			Help:    "Histogram for the duration of the event queue",
			Buckets: utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		EventQueueLength: m.NewGauge(metrics.GaugeOpts{
			Name: "event_queue_length",
			Help: "Gauge for the length of the event queue",
		}),
		Commits: p.Tracer("commits", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []string{},
		})),
		Listens: p.Tracer("listens"),
	}
}
