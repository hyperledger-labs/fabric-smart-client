/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

var logger = flogging.MustGetLogger("fsc.integration")

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

// func NewJaegerExporter(url string) sdktrace.SpanExporter /* (someExporter.Exporter, error) */ {
// 	// Your preferred exporter: console, jaeger, zipkin, OTLP, etc.

// 	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(fmt.Sprintf("http://%s/api/traces", jaeger.WithEndpoint(url)))))
// 	if err != nil {
// 		panic(err)
// 	}
// 	return exporter
// }

// func NewGRPCExporter(url string) sdktrace.SpanExporter /* (someExporter.Exporter, error) */ {
// 	// Your preferred exporter: console, jaeger, zipkin, OTLP, etc.
// 	ctx := context.Background()
// 	// otelAgentAddr, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
// 	// if !ok {
// 	// 	otelAgentAddr = "0.0.0.0:4317"
// 	// }

// 	traceClient := otlptracegrpc.NewClient(
// 		otlptracegrpc.WithInsecure(),
// 		otlptracegrpc.WithEndpoint("http://localhost:4319/v1/traces"),
// 		otlptracegrpc.WithDialOption(grpc.WithBlock()))
// 	sctx, cancel := context.WithTimeout(ctx, time.Second)
// 	defer cancel()

//		traceExp, err := otlptrace.New(sctx, traceClient)
//		handleErr(err, "Failed to create the collector trace exporter")
//		return traceExp
//	}
func NewHTTPExporter(url string, context context.Context) sdktrace.SpanExporter /* (someExporter.Exporter, error) */ {

	traceClient := otlptracehttp.NewClient(
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint(url))

	// sctx, cancel := context.WithTimeout(ctx, time.Second)
	// defer cancel()
	traceExp, err := otlptrace.New(context, traceClient)
	handleErr(err, "Failed to create the collector trace exporter")

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
		panic(err)
	}
	return r

}

func NewTraceProvider(exp sdktrace.SpanExporter, resource *resource.Resource) *sdktrace.TracerProvider {
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource),
	)
	// defer func() {
	// 	if err := tracerProvider.Shutdown(context.Background()); err != nil {
	// 		panic(err)
	// 	}
	// }()

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

	t, ok := h.traces[key]
	if !ok {
		h.handleError("error with tracer: " + h.name + " at event: " + name)
	} else {
		t.span.AddEvent(name, trace.WithTimestamp(timestamp))
	}

}

func (h *LatencyTracer) End(key string, labels ...string) {
	h.EndAt(key, time.Now(), labels...)
}

func (h *LatencyTracer) EndAt(key string, timestamp time.Time, labels ...string) {
	t, ok := h.traces[key]
	if !ok {
		panic("error with tracer: " + key + " at end")
	}
	t.span.End(trace.WithTimestamp(timestamp))
	delete(h.traces, key)
}

func (h *LatencyTracer) handleError(s string) {

}

func handleErr(err error, message string) {
	if err != nil {
		// logger.Info()

	}
}
