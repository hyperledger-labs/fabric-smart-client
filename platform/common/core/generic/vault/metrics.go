/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
)

type Metrics struct {
	CommitDuration        metrics.Histogram
	BatchedCommitDuration metrics.Histogram

	Vault trace.Tracer
}

func NewMetrics(m metrics.Provider, p trace.TracerProvider) *Metrics {
	return &Metrics{
		CommitDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "vault",
			Name:      "commit",
			Help:      "Histogram for the duration of commit",
			Buckets:   utils.ExponentialBucketTimeRange(0, 5*time.Second, 15),
		}),
		BatchedCommitDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "vault",
			Name:      "batched_commit",
			Help:      "Histogram for the duration of commit with the batching overhead",
			Buckets:   utils.ExponentialBucketTimeRange(0, 5*time.Second, 15),
		}),
		Vault: p.Tracer("vault", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "coresdk",
			LabelNames: []tracing.LabelName{},
		})),
	}
}
