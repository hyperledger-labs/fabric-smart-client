/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"context"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc/credentials"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
)

// TracerType names a tracing backend.
type TracerType string

const (
	None        TracerType = "none"
	Otlp        TracerType = "otlp"
	File        TracerType = "file"
	Console     TracerType = "console"
	ServiceName            = "FSC"
)

// NoOp is the configuration for tracing that records nothing.
var NoOp = Config{Provider: None}

// Config is the node's tracing configuration, read from fsc.tracing.
type Config struct {
	Provider TracerType     `mapstructure:"provider"`
	File     FileConfig     `mapstructure:"file"`
	Otlp     OtlpConfig     `mapstructure:"otlp"`
	Sampling SamplingConfig `mapstructure:"sampling"`
}

// SamplingConfig controls what fraction of traces is recorded.
type SamplingConfig struct {
	// Ratio is the fraction of root spans to sample, in [0, 1]. Spans with a remote parent
	// follow the parent's sampling decision, so 0 records only traces sampled upstream.
	Ratio float64 `mapstructure:"ratio"`
}

// FileConfig configures the file backend.
type FileConfig struct {
	Path string `mapstructure:"path"`
}

// OtlpConfig configures the OTLP collector connection.
type OtlpConfig struct {
	Address string `mapstructure:"address"`
	// TLS is the resolved client-side TLS for the collector connection. It has no
	// mapstructure tag: fsc.tracing.otlp.tls is resolved through tlsconfig so an unknown key
	// under it is an error, and so its files are read and validated at startup.
	//
	// TLS is opt-in here. A collector reached over loopback needs none, and requiring it
	// would break every existing tracing setup, so an absent block still means plaintext.
	TLS grpc.SecureOptions `mapstructure:"-"`
}

var logger = logging.MustGetLogger()

// NewProviderFromConfigService returns a tracing provider configured from fsc.tracing,
// resolving the OTLP collector's TLS so a bad block fails here rather than at first export.
// An unset fsc.tracing.sampling.ratio defaults to 1, matching the OpenTelemetry SDK's
// default of sampling every trace.
func NewProviderFromConfigService(confService driver.ConfigService) (Provider, error) {
	c := Config{Sampling: SamplingConfig{Ratio: 1}}
	if err := confService.UnmarshalKey("fsc.tracing", &c); err != nil {
		return nil, err
	}
	// Resolved against itself: the collector is a connection this node dials, and fsc.tls is
	// server-shaped, so there is nothing for it to inherit.
	tlsOpts, err := tlsconfig.ResolveClient(confService, otlpTLSKey)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed resolving %s", otlpTLSKey)
	}
	c.Otlp.TLS = tlsOpts
	return buildProviderFromConfig(c, confService.GetString("fsc.id"))
}

// otlpTLSKey is the configuration subtree holding the OTLP collector connection's TLS.
const otlpTLSKey = "fsc.tracing.otlp.tls"

// NewProviderFromConfig returns a tracing provider from an already-populated [Config],
// including any resolved TLS it carries. Nothing is defaulted: a zero Sampling.Ratio
// samples no root spans.
func NewProviderFromConfig(c Config) (Provider, error) {
	return buildProviderFromConfig(c, ServiceName)
}

// Validate reports whether c names a known provider, matched case-insensitively, with the
// settings that provider needs. A disabled configuration (empty provider or none) is
// always valid.
func (c Config) Validate() error {
	switch c.provider() {
	case "", None:
		return nil
	case Console:
	case File:
		if c.File.Path == "" {
			return errors.New("file provider requires fsc.tracing.file.path")
		}
	case Otlp:
		if c.Otlp.Address == "" {
			return errors.New("otlp provider requires fsc.tracing.otlp.address")
		}
	default:
		return errors.Errorf("unknown fsc.tracing.provider [%s], expected one of [%s, %s, %s, %s]", c.Provider, None, Console, File, Otlp)
	}
	if r := c.Sampling.Ratio; !(r >= 0 && r <= 1) { // also rejects NaN
		return errors.Errorf("fsc.tracing.sampling.ratio [%v] must be in [0, 1]", r)
	}
	return nil
}

func (c Config) provider() TracerType {
	return TracerType(strings.ToLower(string(c.Provider)))
}

func buildProviderFromConfig(c Config, serviceName string) (Provider, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	var exporter sdktrace.SpanExporter
	var err error
	switch c.provider() {
	case Otlp:
		logger.Debugf("OTLP tracer provider selected")
		exporter, err = grpcExporter(&c.Otlp)
	case File:
		logger.Debugf("File tracing provider selected")
		exporter, err = fileExporter(&c.File)
	case Console:
		logger.Debugf("Console tracing provider selected")
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	default: // "" or none: Validate rejects every other value
		logger.Debugf("Tracing disabled")
		return noop.NewTracerProvider(), nil
	}
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to initialize span exporter")
	}
	logger.Debugf("Initializing tracing provider with sampling: %v", c.Sampling)
	return providerWithExporter(context.Background(), exporter, c.Sampling, serviceName)
}

func fileExporter(c *FileConfig) (sdktrace.SpanExporter, error) {
	f, err := os.Create(c.Path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open output file")
	}
	return stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(f))
}

func grpcExporter(c *OtlpConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(c.Address)}
	tlsCfg, err := c.TLS.TLSConfig()
	if err != nil {
		return nil, errors.WithMessage(err, "failed building OTLP exporter TLS")
	}
	if tlsCfg == nil {
		// Unchanged default: TLS here is opt-in, so an absent block keeps the plaintext
		// behaviour every existing tracing setup relies on.
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(tlsCfg)))
	}

	logger.Debugf("Tracing enabled: otlp, TLS [%v]", tlsCfg != nil)
	return otlptrace.New(context.Background(), otlptracegrpc.NewClient(opts...))
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
