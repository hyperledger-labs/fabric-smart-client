/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

// Aliases of the OpenTelemetry option and attribute types, so callers can use
// this package without importing go.opentelemetry.io/otel/trace and attribute.
type (
	SpanStartOption = trace.SpanStartOption
	SpanEndOption   = trace.SpanEndOption
	EventOption     = trace.EventOption
	KeyValue        = attribute.KeyValue
)

// Aliases of the OpenTelemetry option and attribute constructors.
var (
	WithAttributes = trace.WithAttributes
	Bool           = attribute.Bool
	String         = attribute.String
)

// labels holds the metric label values of a span, keyed by the label names the
// tracer declared in its MetricsOpts.
type labels map[string]string

// NewLabels returns labels for the given keys, each set to the empty value.
func NewLabels(keys []string) labels {
	ls := make(labels, len(keys))
	for _, k := range keys {
		ls[k] = ""
	}
	return ls
}

// Append sets the label value of each valid attribute whose key is a declared
// label name, rendering the value with attribute.Value.String (bool true as
// "true", int 3 as "3"). Attributes with any other key are ignored: they stay on
// the trace, but a metric vector rejects label names it was not registered with.
func (l labels) Append(kvs ...attribute.KeyValue) {
	for _, kv := range kvs {
		if _, declared := l[string(kv.Key)]; declared && kv.Valid() {
			l[string(kv.Key)] = kv.Value.String()
		}
	}
}

// ToLabels flattens the labels into the name/value pairs expected by
// metrics.Counter.With and metrics.Histogram.With.
func (l labels) ToLabels() []string {
	r := make([]string, 0, 2*len(l))
	for k, v := range l {
		r = append(r, k, v)
	}
	return r
}

// span wraps a backing trace.Span and records one operation and its duration
// in the tracer's counter and histogram when the span ends, labelled with the
// values collected from the span's attributes. The duration is measured with
// the local clock; timestamps passed with trace.WithTimestamp apply to the
// backing span only.
type span struct {
	trace.Span

	labels     labels
	start      time.Time
	operations metrics.Counter
	duration   metrics.Histogram
}

// End ends the backing span, applies the end attributes to the labels, and
// records the operation and its duration since the span started.
func (s *span) End(options ...SpanEndOption) {
	s.Span.End(options...)

	c := trace.NewSpanEndConfig(options...)
	s.labels.Append(c.Attributes()...)

	ls := s.labels.ToLabels()
	s.operations.With(ls...).Add(1)
	s.duration.With(ls...).Observe(time.Since(s.start).Seconds())
}

// SetAttributes sets the attributes on the backing span and applies them to the
// labels recorded when the span ends.
func (s *span) SetAttributes(kv ...KeyValue) {
	s.Span.SetAttributes(kv...)

	s.labels.Append(kv...)
}

// newSpan returns a span over backingSpan whose labels are labelNames, set from
// the start attributes in opts.
func newSpan(backingSpan trace.Span, labelNames []LabelName, operations metrics.Counter, delay metrics.Histogram, opts ...SpanStartOption) *span {
	c := trace.NewSpanStartConfig(opts...)
	s := &span{
		Span:       backingSpan,
		labels:     NewLabels(labelNames),
		start:      time.Now(),
		operations: operations,
		duration:   delay,
	}
	s.labels.Append(c.Attributes()...)
	return s
}
