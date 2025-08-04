/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
)

type Metrics struct {
	NotifyStatusDuration   metrics.Histogram
	NotifyFinalityDuration metrics.Histogram
	PostFinalityDuration   metrics.Histogram
	HandlerDuration        metrics.Histogram
	EventQueueDuration     metrics.Histogram
	BlockCommitDuration    metrics.Histogram
	EventQueueLength       metrics.Gauge

	Commits trace.Tracer
	Listens trace.Tracer
}

func NewMetrics(p tracing.Provider, m metrics.Provider) *Metrics {
	return &Metrics{
		NotifyStatusDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "notify_status",
			Help:      "Histogram for the duration of notifyStatus",
			Buckets:   utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		NotifyFinalityDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "notify_finality",
			Help:      "Histogram for the duration of notifyFinality",
			Buckets:   utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		PostFinalityDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "post_finality",
			Help:      "Histogram for the duration of postFinality",
			Buckets:   utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		HandlerDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace:  "committer",
			Name:       "handler",
			Help:       "Histogram for the duration of the committer handler",
			LabelNames: []string{"status"},
			Buckets:    utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		BlockCommitDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "block_commit",
			Help:      "Histogram for the duration of the block commit",
			Buckets:   utils.ExponentialBucketTimeRange(0, 5*time.Second, 15),
		}),
		EventQueueDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "event_queue",
			Help:      "Histogram for the duration of the event queue",
			Buckets:   utils.ExponentialBucketTimeRange(0, 1*time.Second, 10),
		}),
		EventQueueLength: m.NewGauge(metrics.GaugeOpts{
			Namespace: "committer",
			Name:      "event_queue_length",
			Help:      "Gauge for the length of the event queue",
		}),
		Commits: p.Tracer("commits", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []string{},
		})),
		Listens: p.Tracer("listens"),
	}
}
