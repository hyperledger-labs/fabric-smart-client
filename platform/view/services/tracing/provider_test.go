/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func TestNewProviderWithBackingProvider_Tracer(t *testing.T) {
	t.Parallel()

	mp := &disabled.Provider{}
	bp := noop.NewTracerProvider()

	p := tracing.NewProviderWithBackingProvider(bp, mp)
	require.NotNil(t, p)

	tracer := p.Tracer("test-tracer", tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace:  "test",
		Subsystem:  "sub",
		LabelNames: []string{"label1"},
	}))
	require.NotNil(t, tracer)
}

func TestNewProviderWithBackingProvider_Tracer_EmptyNamePanics(t *testing.T) {
	t.Parallel()

	mp := &disabled.Provider{}
	bp := noop.NewTracerProvider()
	p := tracing.NewProviderWithBackingProvider(bp, mp)

	require.Panics(t, func() {
		p.Tracer("")
	})
}

func TestNewProviderWithNodeName(t *testing.T) {
	t.Parallel()

	bp := noop.NewTracerProvider()

	p := tracing.NewProviderWithNodeName(bp, "node-1")
	require.NotNil(t, p)

	tracer := p.Tracer("test", tracing.WithMetricsOpts(tracing.MetricsOpts{}))
	require.NotNil(t, tracer)
}
