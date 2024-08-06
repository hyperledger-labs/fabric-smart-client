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

var NoOp = Config{Provider: None}

type Config struct {
	Provider TracerType     `mapstructure:"provider"`
	File     FileConfig     `mapstructure:"file"`
	Otpl     OtplConfig     `mapstructure:"optl"`
	Sampling SamplingConfig `mapstructure:"sampling"`
}

type SamplingConfig struct {
	Ratio float64 `mapstructure:"ratio"`
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
	return newTracerProviderFromConfig(c, confService.GetString("fsc.id"))
}

func NewTracerProviderFromConfig(c Config) (trace.TracerProvider, error) {
	return newTracerProviderFromConfig(c, ServiceName)
}

func newTracerProviderFromConfig(c Config, serviceName string) (trace.TracerProvider, error) {
	var exporter sdktrace.SpanExporter
	var err error
	switch c.Provider {
	case Otpl:
		logger.Infof("OPTL tracer provider selected")
		exporter, err = grpcExporter(&c.Otpl)
	case File:
		logger.Infof("File tracing provider selected")
		exporter, err = fileExporter(&c.File)
	case Console:
		logger.Infof("Console tracing provider selected")
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	case None:
	default:
		logger.Infof("No provider or no-op provider type passed. Tracing disabled.")
		return noop.NewTracerProvider(), nil
	}

	if err != nil {
		return nil, errors.WithMessagef(err, "failed to initialize span exporter")
	}
	logger.Infof("Initializing tracing provider with sampling: %v", c.Sampling)
	return providerWithExporter(context.Background(), exporter, c.Sampling, serviceName)
}

func fileExporter(c *FileConfig) (sdktrace.SpanExporter, error) {
	if c == nil || len(c.Path) == 0 {
		return nil, errors.New("filepath must not be empty")
	}
	f, err := os.Create(c.Path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open output file")
	}
	return stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(f))
}

func grpcExporter(c *OtplConfig) (sdktrace.SpanExporter, error) {
	if c == nil || len(c.Address) == 0 {
		return nil, errors.New("empty url")
	}
	logger.Infof("Tracing enabled: optl")
	return otlptrace.New(context.Background(), otlptracegrpc.NewClient(otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(c.Address)))
}

func providerWithExporter(ctx context.Context, exporter sdktrace.SpanExporter, sampling SamplingConfig, serviceName string) (*sdktrace.TracerProvider, error) {
	// Ensure default SDK resources and the required service name are set.
	r, err := resource.New(ctx, resource.WithAttributes(
		// the service name used to display traces in backends
		semconv.ServiceNameKey.String(serviceName),
	))
	if err != nil {
		return nil, errors.WithMessage(err, "failed creating resource")
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithExportTimeout(1*time.Second)),
		sdktrace.WithResource(r),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampling.Ratio))),
	)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetTracerProvider(tracerProvider)
	return tracerProvider, nil
}
