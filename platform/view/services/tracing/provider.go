/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

// Provider is an alias for tracing.Provider
type Provider = trace.TracerProvider

type metricsProvider interface {
	NewCounter(opts metrics.CounterOpts) metrics.Counter
	NewHistogram(opts metrics.HistogramOpts) metrics.Histogram
}

type providerWithNodeName struct {
	Provider
	nodeName string
}

func NewProviderWithNodeName(p Provider, nodeName string) Provider {
	return &providerWithNodeName{
		Provider: p,
		nodeName: nodeName,
	}
}

func (p *providerWithNodeName) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	c := trace.NewTracerConfig(options...)

	attrs := c.InstrumentationAttributes()
	return p.Provider.Tracer(name, append(options, trace.WithInstrumentationAttributes(append(attrs.ToSlice(), attribute.String(nodeNameKey, p.nodeName))...))...)
}

func NewProvider(confService driver.ConfigService, metricsProvider metrics.Provider) (Provider, error) {
	backingProvider, err := NewProviderFromConfigService(confService)
	if err != nil {
		return nil, err
	}
	return NewProviderWithBackingProvider(backingProvider, metricsProvider), nil
}

func NewProviderWithBackingProvider(tp Provider, mp metricsProvider) Provider {
	return &tracerProvider{metricsProvider: mp, backingProvider: tp}
}

type tracerProvider struct {
	embedded.TracerProvider

	metricsProvider metricsProvider
	backingProvider Provider
}

func (p *tracerProvider) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	if len(name) == 0 {
		panic("tracer name cannot be empty")
	}
	c := trace.NewTracerConfig(options...)

	opts := extractMetricsOpts(c.InstrumentationAttributes())
	return &tracer{
		backingTracer: p.backingProvider.Tracer(name, options...),
		namespace:     opts.Namespace,
		labelNames:    opts.LabelNames,
		operations: p.metricsProvider.NewCounter(metrics.CounterOpts{
			Namespace:    opts.Namespace,
			Subsystem:    opts.Subsystem,
			Name:         fmt.Sprintf("%s_operations", name),
			Help:         fmt.Sprintf("Counter of '%s' operations", name),
			LabelNames:   opts.LabelNames,
			StatsdFormat: statsdFormat(opts.LabelNames),
		}),
		duration: p.metricsProvider.NewHistogram(metrics.HistogramOpts{
			Namespace:    opts.Namespace,
			Subsystem:    opts.Subsystem,
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

// This wrapper is needed in order to be able to fetch the provider using the SP from the Node
var providerType = reflect.TypeOf((*Provider)(nil))

// GetProvider returns the Provider from the passed services.Provider.
// It returns an error if Provider does not exit.
func GetProvider(sp services.Provider) (Provider, error) {
	s, err := sp.GetService(providerType)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting provider")
	}
	return s.(Provider), nil
}
