/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
)

type Metrics struct {
	Vault trace.Tracer
}

func NewMetrics(p trace.TracerProvider) *Metrics {
	return &Metrics{
		Vault: p.Tracer("vault", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "coresdk",
			LabelNames: []tracing.LabelName{},
		})),
	}
}
