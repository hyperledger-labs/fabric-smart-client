/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

type metricsProvider interface {
	NewCounter(opts metrics.CounterOpts) metrics.Counter
	NewHistogram(opts metrics.HistogramOpts) metrics.Histogram
}

type providerWithNodeName struct {
	trace.TracerProvider
	nodeName string
}

func NewProviderWithNodeName(p trace.TracerProvider, nodeName string) trace.TracerProvider {
	return &providerWithNodeName{
		TracerProvider: p,
		nodeName:       nodeName,
	}
}

func (p *providerWithNodeName) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	c := trace.NewTracerConfig(options...)

	attrs := c.InstrumentationAttributes()
	return p.TracerProvider.Tracer(name, append(options, trace.WithInstrumentationAttributes(append(attrs.ToSlice(), attribute.String(nodeNameKey, p.nodeName))...))...)
}

func NewTracerProvider(confService driver.ConfigService, metricsProvider metrics.Provider) (trace.TracerProvider, error) {
	backingProvider, err := tracing.NewTracerProvider(confService)
	if err != nil {
		return nil, err
	}
	return NewTracerProviderWithBackingProvider(backingProvider, metricsProvider), nil
}

func NewTracerProviderWithBackingProvider(tp trace.TracerProvider, mp metricsProvider) trace.TracerProvider {
	return &tracerProvider{metricsProvider: mp, backingProvider: tp}
}

type tracerProvider struct {
	embedded.TracerProvider

	metricsProvider metricsProvider
	backingProvider trace.TracerProvider
}

func (p *tracerProvider) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	c := trace.NewTracerConfig(options...)

	opts := extractMetricsOpts(c.InstrumentationAttributes())
	return &tracer{
		backingTracer: p.backingProvider.Tracer(name, options...),
		namespace:     opts.Namespace,
		labelNames:    opts.LabelNames,
		operations: p.metricsProvider.NewCounter(metrics.CounterOpts{
			Namespace:    opts.Namespace,
			Name:         fmt.Sprintf("%s_operations", name),
			Help:         fmt.Sprintf("Counter of '%s' operations", name),
			LabelNames:   opts.LabelNames,
			StatsdFormat: statsdFormat(opts.LabelNames),
		}),
		duration: p.metricsProvider.NewHistogram(metrics.HistogramOpts{
			Namespace:    opts.Namespace,
			Name:         fmt.Sprintf("%s_duration", name),
			Help:         fmt.Sprintf("Histogram for the duration of '%s' operations", name),
			LabelNames:   opts.LabelNames,
			StatsdFormat: statsdFormat(opts.LabelNames),
		}),
	}
}

func statsdFormat(labels []LabelName) string {
	return "%{#fqname}.%{" + strings.Join(labels, "}.%{") + "}"
}
