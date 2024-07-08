/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type (
	SpanStartOption = trace.SpanStartOption
	SpanEndOption   = trace.SpanEndOption
	EventOption     = trace.EventOption
	KeyValue        = attribute.KeyValue
)

var (
	WithTimestamp  = trace.WithTimestamp
	WithAttributes = trace.WithAttributes
	Int            = attribute.Int
	Bool           = attribute.Bool
	String         = attribute.String
)

type labels map[string]string

func NewLabels(keys []string) labels {
	ls := make(labels, len(keys))
	for _, k := range keys {
		ls[k] = ""
	}
	return ls
}

func (l labels) Append(kvs ...attribute.KeyValue) {
	for _, kv := range kvs {
		if kv.Valid() {
			l[string(kv.Key)] = kv.Value.AsString()
		}
	}
}

func (l labels) ToLabels() []string {
	r := make([]string, 0, 2*len(l))
	for k, v := range l {
		r = append(r, k, v)
	}
	return r
}

type span struct {
	trace.Span

	labels     labels
	start      time.Time
	operations metrics.Counter
	duration   metrics.Histogram
}

func (s *span) End(options ...SpanEndOption) {
	s.Span.End(options...)

	c := trace.NewSpanEndConfig(options...)
	s.labels.Append(c.Attributes()...)

	ls := s.labels.ToLabels()
	s.operations.With(ls...).Add(1)
	s.duration.With(ls...).Observe(defaultNow(c.Timestamp()).Sub(s.start).Seconds())
}

func (s *span) AddEvent(name string, options ...EventOption) {
	s.Span.AddEvent(name, options...)

	c := trace.NewEventConfig(options...)
	s.labels.Append(c.Attributes()...)
}

func (s *span) SetAttributes(kv ...KeyValue) {
	s.Span.SetAttributes(kv...)

	s.labels.Append(kv...)
}

func newSpan(backingSpan trace.Span, labelNames []LabelName, operations metrics.Counter, delay metrics.Histogram, opts ...SpanStartOption) *span {
	c := trace.NewSpanStartConfig(opts...)
	s := &span{
		Span:       backingSpan,
		labels:     NewLabels(labelNames),
		start:      defaultNow(c.Timestamp()),
		operations: operations,
		duration:   delay,
	}
	s.labels.Append(c.Attributes()...)
	return s
}

func defaultNow(t time.Time) time.Time {
	//if t.IsZero() {
	//	return time.Now()
	//}
	return time.Now()
}
