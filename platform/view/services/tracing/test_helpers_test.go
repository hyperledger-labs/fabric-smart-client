/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

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
