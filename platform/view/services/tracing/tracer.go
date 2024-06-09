/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

type tracer struct {
	embedded.Tracer
	backingTracer trace.Tracer

	labelNames []LabelName
	operations metrics.Counter
	duration   metrics.Histogram
}

func (t *tracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	newCtx, backingSpan := t.backingTracer.Start(ctx, spanName, opts...)

	return newCtx, newSpan(backingSpan, t.labelNames, t.operations, t.duration, opts...)
}
