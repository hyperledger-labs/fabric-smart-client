/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// recordingCounter captures the label values passed to With for later assertion.
type recordingCounter struct {
	lastLabels []string
	addCalls   int
}

func (c *recordingCounter) With(labelValues ...string) metrics.Counter {
	c.lastLabels = append([]string(nil), labelValues...)
	return c
}

func (c *recordingCounter) Add(delta float64) {
	c.addCalls++
}

// recordingHistogram captures the label values passed to With for later assertion.
type recordingHistogram struct {
	lastLabels   []string
	observeCalls int
}

func (h *recordingHistogram) With(labelValues ...string) metrics.Histogram {
	h.lastLabels = append([]string(nil), labelValues...)
	return h
}

func (h *recordingHistogram) Observe(value float64) {
	h.observeCalls++
}

// recordingMetricsProvider returns recording Counter and Histogram instances so tests
// can assert on the labels that flushed at span end.
type recordingMetricsProvider struct {
	counter   *recordingCounter
	histogram *recordingHistogram
}

func newRecordingMetricsProvider() *recordingMetricsProvider {
	return &recordingMetricsProvider{
		counter:   &recordingCounter{},
		histogram: &recordingHistogram{},
	}
}

func (p *recordingMetricsProvider) NewCounter(_ metrics.CounterOpts) metrics.Counter {
	return p.counter
}

func (p *recordingMetricsProvider) NewGauge(_ metrics.GaugeOpts) metrics.Gauge {
	// Not used by the tracer; return nil so a nil-deref surfaces if that changes.
	return nil
}

func (p *recordingMetricsProvider) NewHistogram(_ metrics.HistogramOpts) metrics.Histogram {
	return p.histogram
}

func TestTracer_Start(t *testing.T) {
	t.Parallel()

	mp := &disabled.Provider{}
	bp := noop.NewTracerProvider()
	p := tracing.NewProviderWithBackingProvider(bp, mp)

	tracer := p.Tracer("test-tracer", tracing.WithMetricsOpts(tracing.MetricsOpts{
		LabelNames: []string{"op"},
	}))

	ctx, span := tracer.Start(
		context.Background(), "my-span",
		tracing.WithAttributes(tracing.String("op", "read")),
	)
	require.NotNil(t, ctx)
	require.NotNil(t, span)

	span.End()
}

func TestTracer_Start_SetAttributes_FlushesLabelsAtEnd(t *testing.T) {
	t.Parallel()

	// Use a recording metrics provider so we can observe the labels that the span
	// flushes to the metrics counter/histogram when End is called.
	mp := newRecordingMetricsProvider()
	bp := noop.NewTracerProvider()
	p := tracing.NewProviderWithBackingProvider(bp, mp)

	tracer := p.Tracer("test-tracer", tracing.WithMetricsOpts(tracing.MetricsOpts{
		LabelNames: []string{"op"},
	}))

	_, span := tracer.Start(context.Background(), "my-span")
	span.SetAttributes(tracing.String("op", "write"))
	span.End()

	// SetAttributes must reach the labels map, which End flushes into the counter
	// and histogram via With(labelValues...).
	require.Contains(t, mp.counter.lastLabels, "op")
	require.Contains(t, mp.counter.lastLabels, "write")
	require.Equal(t, 1, mp.counter.addCalls)

	require.Contains(t, mp.histogram.lastLabels, "op")
	require.Contains(t, mp.histogram.lastLabels, "write")
	require.Equal(t, 1, mp.histogram.observeCalls)
}

func TestTracer_Start_AddEvent_DelegatesToBackingSpan(t *testing.T) {
	t.Parallel()

	// AddEvent on our span is a direct delegation to the underlying trace.Span.
	// We use a recording backing tracer to verify the call reaches the underlying
	// span with the expected name.
	rt := newRecordingTracer()
	bp := newRecordingTracerProvider(rt)
	mp := &disabled.Provider{}
	p := tracing.NewProviderWithBackingProvider(bp, mp)

	tracer := p.Tracer("test-tracer", tracing.WithMetricsOpts(tracing.MetricsOpts{}))

	_, span := tracer.Start(context.Background(), "my-span")
	span.AddEvent("my-event")
	span.End()

	require.Len(t, rt.spans, 1)
	require.Equal(t, []string{"my-event"}, rt.spans[0].events)
}
