/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func TestProviderWithNodeName(t *testing.T) {
	t.Parallel()

	provider := tracing.NewProviderWithNodeName(newTestProvider(t), "test-node")
	require.NotNil(t, provider)

	tracer := provider.Tracer("test-tracer")
	require.NotNil(t, tracer)
}

func TestProviderWithNodeName_TracerIncludesNodeName(t *testing.T) {
	t.Parallel()

	provider := tracing.NewProviderWithNodeName(newTestProvider(t), "my-node-123")
	tracer := provider.Tracer("test-tracer")

	ctx, span := tracer.Start(t.Context(), "test-span")
	defer span.End()

	require.NotNil(t, ctx)
	require.NotNil(t, span)
}

func TestNewProviderWithBackingProvider(t *testing.T) {
	t.Parallel()

	provider := newTestProvider(t)
	require.NotNil(t, provider)

	tracer := provider.Tracer("test-tracer")
	require.NotNil(t, tracer)
}

func TestTracerProvider_Tracer_EmptyName(t *testing.T) {
	t.Parallel()

	provider := newTestProvider(t)

	require.Panics(t, func() {
		provider.Tracer("")
	})
}

func TestTracerProvider_TracerWithMetricsOpts(t *testing.T) {
	t.Parallel()

	provider := newTestProvider(t)

	opts := tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace:  "test_ns",
		Subsystem:  "test_sub",
		LabelNames: []string{"label1", "label2"},
	})

	tracer := provider.Tracer("test-tracer", opts)
	require.NotNil(t, tracer)

	ctx, span := tracer.Start(t.Context(), "test-span")
	defer span.End()

	require.NotNil(t, ctx)
	require.NotNil(t, span)
}

type mockMetricsProvider struct {
	counters   map[string]*mockCounter
	histograms map[string]*mockHistogram
}

type mockCounter struct {
	values map[string]float64
}

type mockHistogram struct {
	values map[string][]float64
}

func (c *mockCounter) Add(delta float64) {
	c.values["total"] += delta
}

func (c *mockCounter) With(_ ...string) metrics.Counter {
	return c
}

func (h *mockHistogram) Observe(value float64) {
	h.values["observations"] = append(h.values["observations"], value)
}

func (h *mockHistogram) With(_ ...string) metrics.Histogram {
	return h
}

func (mp *mockMetricsProvider) NewCounter(opts metrics.CounterOpts) metrics.Counter {
	key := fmt.Sprintf("%s_%s_%s", opts.Namespace, opts.Subsystem, opts.Name)
	c := &mockCounter{values: make(map[string]float64)}
	mp.counters[key] = c
	return c
}

func (mp *mockMetricsProvider) NewHistogram(opts metrics.HistogramOpts) metrics.Histogram {
	key := fmt.Sprintf("%s_%s_%s", opts.Namespace, opts.Subsystem, opts.Name)
	h := &mockHistogram{values: make(map[string][]float64)}
	mp.histograms[key] = h
	return h
}

func TestTracerProvider_CompleteTraceLifecycle(t *testing.T) {
	t.Parallel()

	metricsProvider := &mockMetricsProvider{
		counters:   make(map[string]*mockCounter),
		histograms: make(map[string]*mockHistogram),
	}

	provider := tracing.NewProviderWithBackingProvider(newTestProvider(t), metricsProvider)

	opts := tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace:  "test_ns",
		Subsystem:  "test_sub",
		LabelNames: []string{"status"},
	})

	tracer := provider.Tracer("test-tracer", opts)
	ctx := t.Context()

	ctx, span := tracer.Start(ctx, "operation", tracing.WithAttributes(tracing.String("status", "success")))
	require.NotNil(t, ctx)
	require.NotNil(t, span)

	span.SetAttributes(tracing.String("status", "completed"))
	span.AddEvent("event1", tracing.WithAttributes(tracing.String("type", "test")))
	span.End()

	require.NotEmpty(t, metricsProvider.counters)
	require.NotEmpty(t, metricsProvider.histograms)
}

func TestGetProvider_Success(t *testing.T) {
	t.Parallel()

	provider := newTestProvider(t)

	retrievedProvider, err := tracing.GetProvider(mockServiceProvider{provider: provider})
	require.NoError(t, err)
	require.Equal(t, provider, retrievedProvider)
}

type mockServiceProvider struct {
	provider trace.TracerProvider
}

func (m mockServiceProvider) GetService(serviceType interface{}) (interface{}, error) {
	return m.provider, nil
}

type failingServiceProvider struct{}

func (f failingServiceProvider) GetService(serviceType interface{}) (interface{}, error) {
	return nil, fmt.Errorf("service not found")
}

func TestGetProvider_Error(t *testing.T) {
	t.Parallel()

	_, err := tracing.GetProvider(failingServiceProvider{})
	require.Error(t, err)
}
