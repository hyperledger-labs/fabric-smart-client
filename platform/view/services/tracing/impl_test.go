/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func TestNewProviderFromConfig_Disabled(t *testing.T) {
	t.Parallel()

	for _, provider := range []tracing.TracerType{"", tracing.None, "NONE"} {
		p, err := tracing.NewProviderFromConfig(tracing.Config{Provider: provider})
		require.NoError(t, err)
		require.IsType(t, noop.TracerProvider{}, p, "provider [%s]", provider)
	}
}

func TestNewProviderFromConfig_RatioOutOfRange(t *testing.T) {
	t.Parallel()

	for _, ratio := range []float64{-0.1, 1.5, math.NaN()} {
		_, err := tracing.NewProviderFromConfig(tracing.Config{Provider: tracing.Console, Sampling: tracing.SamplingConfig{Ratio: ratio}})
		require.ErrorContains(t, err, "must be in [0, 1]", "ratio [%v]", ratio)
	}
}

// TestNewProviderFromConfigService_SamplingRatio checks that an unset
// fsc.tracing.sampling.ratio samples every root span, while an explicit 0 is kept.
func TestNewProviderFromConfigService_SamplingRatio(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		yaml    string
		sampled bool
	}{
		"unset":       {yaml: "provider: Console", sampled: true},
		"empty block": {yaml: "provider: console\n    sampling: {}", sampled: true},
		"explicit 0":  {yaml: "provider: console\n    sampling:\n      ratio: 0", sampled: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p, err := providerFromYAML(t, tc.yaml)
			require.NoError(t, err)
			_, span := p.Tracer("test").Start(t.Context(), "root")
			defer span.End()
			require.Equal(t, tc.sampled, span.SpanContext().IsSampled())
		})
	}
}

// TestUnmarshalTracingConfig_SkipsOTLPTLS checks that fsc.tracing.otlp.tls is not
// decoded into OtlpConfig.TLS. That block is resolved later by tlsconfig.
func TestUnmarshalTracingConfig_SkipsOTLPTLS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	core := "fsc:\n  tracing:\n    provider: otlp\n    otlp:\n      address: 127.0.0.1:4317\n      tls:\n        enabled: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(core), 0o600))
	cs, err := config.NewProvider(dir)
	require.NoError(t, err)

	var c tracing.Config
	require.NoError(t, cs.UnmarshalKey("fsc.tracing", &c))
	require.Equal(t, tracing.Otlp, c.Provider)
	require.Equal(t, "127.0.0.1:4317", c.Otlp.Address)
	require.Zero(t, c.Otlp.TLS)
}

func TestNewProviderFromConfigService_UnknownProvider(t *testing.T) {
	t.Parallel()

	for _, provider := range []string{"otel", "jaeger"} {
		_, err := providerFromYAML(t, "provider: "+provider)
		require.ErrorContains(t, err, "unknown fsc.tracing.provider ["+provider+"]")
	}
}

// providerFromYAML builds a provider from a core.yaml with tracingYAML as its fsc.tracing block.
func providerFromYAML(t *testing.T, tracingYAML string) (tracing.Provider, error) {
	t.Helper()
	dir := t.TempDir()
	core := "fsc:\n  tracing:\n    " + tracingYAML + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(core), 0o600))
	cs, err := config.NewProvider(dir)
	require.NoError(t, err)
	return tracing.NewProviderFromConfigService(cs)
}

func TestNewProviderFromConfig_Console(t *testing.T) {
	t.Parallel()

	p, err := tracing.NewProviderFromConfig(tracing.Config{
		Provider: tracing.Console,
		Sampling: tracing.SamplingConfig{Ratio: 1.0},
	})
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNewProviderFromConfig_File(t *testing.T) {
	t.Parallel()

	// t.TempDir returns a directory that is automatically cleaned up when the
	// test finishes, even on panic. No manual defer/Remove needed.
	dir := t.TempDir()
	path := filepath.Join(dir, "tracing-test.json")

	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	p, err := tracing.NewProviderFromConfig(tracing.Config{
		Provider: tracing.File,
		File:     tracing.FileConfig{Path: path},
		Sampling: tracing.SamplingConfig{Ratio: 1.0},
	})
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNewProviderFromConfig_File_EmptyPath(t *testing.T) {
	t.Parallel()

	_, err := tracing.NewProviderFromConfig(tracing.Config{
		Provider: tracing.File,
		File:     tracing.FileConfig{Path: ""},
	})
	require.Error(t, err)
}

func TestNewProviderFromConfig_Otlp_EmptyAddress(t *testing.T) {
	t.Parallel()

	_, err := tracing.NewProviderFromConfig(tracing.Config{
		Provider: tracing.Otlp,
		Otlp:     tracing.OtlpConfig{Address: ""},
	})
	require.Error(t, err)
}
