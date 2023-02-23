/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package optl

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	logger = flogging.MustGetLogger("tracing")
)

type MyKey string

type traceData struct {
	start time.Time
	span  trace.Span
}

type Tracer struct {
	name    string
	enabled bool
	tracer  trace.Tracer

	tracesLock sync.RWMutex
	traces     map[string]*traceData
}

type LatencyTracerOpts struct {
	Name    string
	Version string
}

func NewLatencyTracer(tp trace.TracerProvider, opts LatencyTracerOpts) *Tracer {
	return &Tracer{
		name:    opts.Name,
		enabled: true,
		tracer:  tp.Tracer(opts.Name),
		traces:  make(map[string]*traceData),
	}
}

func (h *Tracer) Start(spanName string) {
	h.StartAt(spanName, time.Now())
}

func (h *Tracer) StartAt(spanName string, when time.Time) {
	ctx := context.WithValue(context.Background(), MyKey(spanName), spanName)
	_, span := h.tracer.Start(ctx, spanName, trace.WithTimestamp(when), trace.WithAttributes(attribute.String("id", spanName)))

	h.tracesLock.Lock()
	defer h.tracesLock.Unlock()
	h.traces[spanName] = &traceData{when, span}
}

func (h *Tracer) AddEvent(spanName string, eventName string) {
	h.AddEventAt(spanName, eventName, time.Now())
}

func (h *Tracer) AddEventAt(spanName string, eventName string, when time.Time) {
	t := getTraceData(h, spanName)
	if t == nil {
		return
	}
	t.span.AddEvent(eventName, trace.WithTimestamp(when))
}

func (h *Tracer) End(spanName string, labels ...string) {
	h.EndAt(spanName, time.Now(), labels...)
}

func (h *Tracer) EndAt(spanName string, when time.Time, labels ...string) {
	t := getTraceData(h, spanName)
	if t == nil {
		return
	}
	t.span.End(trace.WithTimestamp(when))

	h.tracesLock.Lock()
	defer h.tracesLock.Unlock()
	delete(h.traces, spanName)
}

func (h *Tracer) AddError(spanName string, err error) {
	t := getTraceData(h, spanName)
	if t == nil {
		return
	}
	t.span.RecordError(err)
}

func getTraceData(h *Tracer, key string) *traceData {
	h.tracesLock.RLock()
	defer h.tracesLock.RUnlock()

	t, ok := h.traces[key]
	if !ok {
		logger.Errorf("error with tracer: " + key + " at end")
	}
	return t
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

func LaunchOptl(url string, context context.Context) *sdktrace.TracerProvider {
	r := newResource(context)
	tp := NewTraceProvider(NewHTTPExporter(url, context), r)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetTracerProvider(tp)
	return tp
}
