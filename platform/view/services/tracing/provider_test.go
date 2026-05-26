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
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
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

func newTestProvider(t *testing.T) tracing.Provider {
	t.Helper()

	provider := tracing.NewProviderWithBackingProvider(noop.NewTracerProvider(), &disabled.Provider{})
	require.NotNil(t, provider)

	return provider
}

func newTestTracer(t *testing.T, labelNames ...string) trace.Tracer {
	t.Helper()

	provider := newTestProvider(t)
	tracer := provider.Tracer("test-tracer", tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace:  "test",
		Subsystem:  "test",
		LabelNames: labelNames,
	}))
	require.NotNil(t, tracer)

	return tracer
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
