/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type traceData struct {
	start time.Time
	span  trace.Span
}

type LatencyTracer struct {
	name    string
	enabled bool
	tracer  trace.Tracer
	traces  map[string]*traceData
}

type LatencyTracerOpts struct {
	Name    string
	Version string
}

func NewLatencyTracer(tp trace.TracerProvider, opts LatencyTracerOpts) *LatencyTracer {
	return &LatencyTracer{
		name:    opts.Name,
		enabled: true,
		tracer:  tp.Tracer(opts.Name),
		traces:  make(map[string]*traceData),
	}
}

func NewHTTPExporter(url string, context context.Context) sdktrace.SpanExporter /* (someExporter.Exporter, error) */ {

	traceClient := otlptracehttp.NewClient(
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint(url))

	traceExp, err := otlptrace.New(context, traceClient)
	if err != nil {
		logger.Error(err, "Failed to create the collector trace exporter")
	}

	return traceExp
}

func newResource(context context.Context) *resource.Resource {
	// Ensure default SDK resources and the required service name are set.
	r, err := resource.New(context,
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String("FSC"),
		),
	)
	if err != nil {
		logger.Error(err)
	}
	return r

}

func NewTraceProvider(exp sdktrace.SpanExporter, resource *resource.Resource) *sdktrace.TracerProvider {
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource),
	)
	return tracerProvider
}

func (h *LatencyTracer) Start(key string) {
	h.StartAt(key, time.Now())
}

func (h *LatencyTracer) StartAt(key string, timestamp time.Time) {
	ctx := context.WithValue(context.Background(), key, key)
	_, span := h.tracer.Start(ctx, key, trace.WithTimestamp(timestamp), trace.WithAttributes(attribute.String("id", key)))
	h.traces[key] = &traceData{timestamp, span}
}

func (h *LatencyTracer) AddEvent(key string, name string) {
	h.AddEventAt(key, name, time.Now())
}

func (h *LatencyTracer) AddEventAt(key string, name string, timestamp time.Time) {
	t := getTraceData(h, key)
	t.span.AddEvent(name, trace.WithTimestamp(timestamp))
}

func (h *LatencyTracer) End(key string, labels ...string) {
	h.EndAt(key, time.Now(), labels...)
}

func (h *LatencyTracer) EndAt(key string, timestamp time.Time, labels ...string) {
	t := getTraceData(h, key)
	t.span.End(trace.WithTimestamp(timestamp))
	delete(h.traces, key)
}

func (h *LatencyTracer) AddError(key string, err error) {
	t := getTraceData(h, key)
	t.span.RecordError(err)
}

func getTraceData(h *LatencyTracer, key string) *traceData {
	t, ok := h.traces[key]
	if !ok {
		logger.Errorf("error with tracer: " + key + " at end")
	}
	return t
}
