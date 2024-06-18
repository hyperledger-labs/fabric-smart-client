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
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type TracerType string

const (
	None        TracerType = "none"
	Otpl        TracerType = "otpl"
	File        TracerType = "file"
	Console     TracerType = "console"
	ServiceName            = "FSC"
)

type Config struct {
	Provider TracerType `mapstructure:"provider"`
	File     FileConfig `mapstructure:"file"`
	Otpl     OtplConfig `mapstructure:"optl"`
}

type FileConfig struct {
	Path string `mapstructure:"path"`
}

type OtplConfig struct {
	Address string `mapstructure:"address"`
}

var logger = flogging.MustGetLogger("view-sdk.tracing")

func NewTracerProvider(confService driver.ConfigService) (trace.TracerProvider, error) {
	c := Config{}
	if err := confService.UnmarshalKey("fsc.tracing", &c); err != nil {
		return nil, err
	}
	return NewTracerProviderFromConfig(c)
}

func NewTracerProviderFromConfig(c Config) (trace.TracerProvider, error) {
	switch c.Provider {
	case None:
		logger.Infof("No-op tracer provider selected")
		return NoopProvider()
	case Otpl:
		logger.Infof("OPTL tracer provider selected")
		return GrpcProvider(&c.Otpl)
	case File:
		logger.Infof("File tracing provider selected")
		return FileProvider(&c.File)
	case Console:
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

func FileProvider(c *FileConfig) (*sdktrace.TracerProvider, error) {
	if c == nil || len(c.Path) == 0 {
		return nil, errors.New("filepath must not be empty")
	}
	f, err := os.Create(c.Path)
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

func GrpcProvider(c *OtplConfig) (*sdktrace.TracerProvider, error) {
	if c == nil || len(c.Address) == 0 {
		return nil, errors.New("empty url")
	}
	logger.Infof("Tracing enabled: optl")
	exporter, err := otlptrace.New(context.Background(), otlptracegrpc.NewClient(otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(c.Address)))
	if err != nil {
		return nil, errors.Wrap(err, "failed creating trace exporter")
	}
	return providerWithExporter(context.Background(), exporter)
}

func providerWithExporter(ctx context.Context, exporter sdktrace.SpanExporter) (*sdktrace.TracerProvider, error) {
	// Ensure default SDK resources and the required service name are set.
	r, err := resource.New(ctx, resource.WithAttributes(
		// the service name used to display traces in backends
		semconv.ServiceNameKey.String(ServiceName),
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
