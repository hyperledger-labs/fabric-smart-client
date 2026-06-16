/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

// recordingSpan captures method calls made on a trace.Span so tests can assert
// on the interactions between our tracing wrapper and the underlying span.
type recordingSpan struct {
	embedded.Span

	name       string
	attributes []attribute.KeyValue
	events     []string
	ended      bool
}

func (s *recordingSpan) End(options ...trace.SpanEndOption) { s.ended = true }
func (s *recordingSpan) AddEvent(name string, options ...trace.EventOption) {
	s.events = append(s.events, name)
}
func (s *recordingSpan) AddLink(link trace.Link)                             {}
func (s *recordingSpan) IsRecording() bool                                   { return true }
func (s *recordingSpan) RecordError(err error, options ...trace.EventOption) {}
func (s *recordingSpan) SpanContext() trace.SpanContext                      { return trace.SpanContext{} }
func (s *recordingSpan) SetStatus(code codes.Code, description string)       {}
func (s *recordingSpan) SetName(name string)                                 { s.name = name }
func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.attributes = append(s.attributes, kv...)
}
func (s *recordingSpan) TracerProvider() trace.TracerProvider { return nil }

// recordingTracer creates recordingSpan instances and keeps a reference to each
// one so tests can inspect what happened after Start returned.
type recordingTracer struct {
	embedded.Tracer

	spans []*recordingSpan
}

func newRecordingTracer() *recordingTracer { return &recordingTracer{} }

func (t *recordingTracer) Start(
	ctx context.Context, name string, opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	s := &recordingSpan{name: name}
	t.spans = append(t.spans, s)
	return ctx, s
}

// recordingTracerProvider returns a fixed recordingTracer so tests can attach a
// recorder to the tracing provider under test.
type recordingTracerProvider struct {
	embedded.TracerProvider

	tracer *recordingTracer
}

func newRecordingTracerProvider(rt *recordingTracer) *recordingTracerProvider {
	return &recordingTracerProvider{tracer: rt}
}

func (p *recordingTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return p.tracer
}
