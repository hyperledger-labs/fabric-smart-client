/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
)

const (
	HeaderTypeLabel tracing.LabelName = "header_type"
)

type Metrics struct {
	Commits trace.Tracer
	Listens trace.Tracer
}

func NewMetrics(p trace.TracerProvider) *Metrics {
	return &Metrics{
		Commits: p.Tracer("commits", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "fsc",
			LabelNames: []string{HeaderTypeLabel},
		})),
		Listens: p.Tracer("listens"),
	}
}
