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

	Commits trace.Tracer
	Listens trace.Tracer
}

func NewMetrics(p trace.TracerProvider, m metrics.Provider) *Metrics {
	return &Metrics{
		NotifyStatusDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "notify_status",
			Help:      "Histogram for the duration of notifyStatus",
			Buckets:   utils.ExponentialBucketRange(0, 10*time.Second, 10),
		}),
		NotifyFinalityDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "notify_finality",
			Help:      "Histogram for the duration of notifyFinality",
			Buckets:   utils.ExponentialBucketRange(0, 10*time.Second, 10),
		}),
		PostFinalityDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "post_finality",
			Help:      "Histogram for the duration of postFinality",
			Buckets:   utils.ExponentialBucketRange(0, 10*time.Second, 10),
		}),
		HandlerDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "committer",
			Name:      "handler",
			Help:      "Histogram for the duration of the committer handler",
			Buckets:   utils.ExponentialBucketRange(0, 10*time.Second, 10),
		}),
		Commits: p.Tracer("commits", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "fsc",
			LabelNames: []string{},
		})),
		Listens: p.Tracer("listens"),
	}
}
