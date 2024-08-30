/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
)

type Metrics struct {
	ProcessDuration metrics.Histogram
	ConnectDuration metrics.Histogram
	BlockSize       metrics.Histogram

	BlockHeight metrics.Gauge

	Delivery trace.Tracer
}

func NewMetrics(p trace.TracerProvider, m metrics.Provider) *Metrics {
	return &Metrics{
		ProcessDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "delivery",
			Name:      "callback",
			Help:      "Histogram for the duration of callback",
			Buckets:   utils.ExponentialBucketTimeRange(0, 2*time.Second, 10),
		}),
		ConnectDuration: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "delivery",
			Name:      "connect",
			Help:      "Histogram for the duration of connect",
			Buckets:   utils.ExponentialBucketTimeRange(0, 2*time.Second, 10),
		}),
		BlockSize: m.NewHistogram(metrics.HistogramOpts{
			Namespace: "delivery",
			Name:      "block_size",
			Help:      "Histogram for the block size",
			Buckets:   utils.LinearBucketRange(0, 1000, 20),
		}),
		BlockHeight: m.NewGauge(metrics.GaugeOpts{
			Namespace: "delivery",
			Name:      "block_height",
			Help:      "Gauge for the current block height",
		}),
		Delivery: p.Tracer("delivery", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "fabricsdk",
			LabelNames: []tracing.LabelName{messageTypeLabel},
		})),
	}
}
