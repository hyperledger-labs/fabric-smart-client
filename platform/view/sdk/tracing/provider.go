/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"context"
	"os"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var logger = flogging.MustGetLogger("view-sdk.tracing")

func NewTracerProvider(confService driver.ConfigService) (trace.TracerProvider, error) {
	switch confService.GetString("fsc.tracing.provider") {
	case "none":
		logger.Infof("No-op tracer provider selected")
		return NoopProvider()
	case "optl":
		logger.Infof("OPTL tracer provider selected")
		return HttpProvider(confService.GetString("fsc.tracing.optl.address"))
	case "file":
		logger.Infof("File tracing provider selected")
		return FileProvider(confService.GetPath("fsc.tracing.file.path"))
	case "console":
		logger.Infof("Console tracing provider selected")
		return ConsoleProvider()
	default:
		logger.Infof("No provider type passed. Default to no-op")
		return NoopProvider()
	}
}

func NoopProvider() (noop.TracerProvider, error) {
	logger.Infof("Tracing disabled")
	return noop.NewTracerProvider(), nil
}

func FileProvider(filepath string) (*sdktrace.TracerProvider, error) {
	if len(filepath) == 0 {
		return nil, errors.New("filepath must not be empty")
	}
	f, err := os.Create(filepath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open output file")
	}
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(f))
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize stdouttrace")
	}
	return providerWithExporter(context.Background(), exporter)
}

func ConsoleProvider() (*sdktrace.TracerProvider, error) {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize stdouttrace")
	}
	return providerWithExporter(context.Background(), exporter)
}

func HttpProvider(url string) (*sdktrace.TracerProvider, error) {
	if len(url) == 0 {
		return nil, errors.New("empty url")
	}
	logger.Infof("Tracing enabled: optl")
	exporter, err := otlptrace.New(context.Background(), otlptracehttp.NewClient(otlptracehttp.WithInsecure(), otlptracehttp.WithEndpoint(url)))
	if err != nil {
		return nil, errors.Wrap(err, "failed creating trace exporter")
	}
	return providerWithExporter(context.Background(), exporter)
}

func providerWithExporter(ctx context.Context, exporter sdktrace.SpanExporter) (*sdktrace.TracerProvider, error) {
	// Ensure default SDK resources and the required service name are set.
	r, err := resource.New(ctx, resource.WithAttributes(
		// the service name used to display traces in backends
		semconv.ServiceNameKey.String("FSC"),
	))
	if err != nil {
		return nil, errors.WithMessage(err, "failed creating resource")
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithExportTimeout(1*time.Second)),
		sdktrace.WithResource(r),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(1.0)),
	)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetTracerProvider(tracerProvider)
	return tracerProvider, nil
}
