/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func TestNewProviderFromConfig_NoneProvider(t *testing.T) {
	t.Parallel()

	config := tracing.Config{
		Provider: tracing.None,
	}

	provider, err := tracing.NewProviderFromConfig(config)
	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestNewProviderFromConfig_ConsoleProvider(t *testing.T) {
	t.Parallel()

	config := tracing.Config{
		Provider: tracing.Console,
	}

	provider, err := tracing.NewProviderFromConfig(config)
	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestNewProviderFromConfig_FileProvider(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "traces.txt")

	config := tracing.Config{
		Provider: tracing.File,
		File: tracing.FileConfig{
			Path: filePath,
		},
	}

	provider, err := tracing.NewProviderFromConfig(config)
	require.NoError(t, err)
	require.NotNil(t, provider)

	_, err = os.Stat(filePath)
	require.NoError(t, err)
}

func TestNewProviderFromConfig_FileProvider_EmptyPath(t *testing.T) {
	t.Parallel()

	config := tracing.Config{
		Provider: tracing.File,
		File: tracing.FileConfig{
			Path: "",
		},
	}

	_, err := tracing.NewProviderFromConfig(config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "filepath must not be empty")
}

func TestNewProviderFromConfig_FileProvider_InvalidPath(t *testing.T) {
	t.Parallel()

	config := tracing.Config{
		Provider: tracing.File,
		File: tracing.FileConfig{
			Path: "/invalid/path/that/does/not/exist/traces.txt",
		},
	}

	_, err := tracing.NewProviderFromConfig(config)
	require.Error(t, err)
}

func TestNewProviderFromConfig_OtlpProvider_EmptyAddress(t *testing.T) {
	t.Parallel()

	config := tracing.Config{
		Provider: tracing.Otlp,
		Otlp: tracing.OtlpConfig{
			Address: "",
		},
	}

	_, err := tracing.NewProviderFromConfig(config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty url")
}

func TestNewProviderFromConfig_OtlpProvider(t *testing.T) {
	t.Parallel()

	t.Skip("Skipping OTLP test as it requires external service")

	config := tracing.Config{
		Provider: tracing.Otlp,
		Otlp: tracing.OtlpConfig{
			Address: "localhost:4317",
		},
		Sampling: tracing.SamplingConfig{
			Ratio: 1.0,
		},
	}

	provider, err := tracing.NewProviderFromConfig(config)
	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestNewProviderFromConfig_WithSampling(t *testing.T) {
	t.Parallel()

	config := tracing.Config{
		Provider: tracing.Console,
		Sampling: tracing.SamplingConfig{
			Ratio: 0.5,
		},
	}

	provider, err := tracing.NewProviderFromConfig(config)
	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestRegisterReplacer(t *testing.T) {
	t.Parallel()

	key := "test_replacer_key_" + randomSuffix()
	tracing.RegisterReplacer(key, "replacement_value")

	replacers := tracing.Replacers()
	require.Contains(t, replacers, key)
	require.Equal(t, "replacement_value", replacers[key])
}

func TestRegisterReplacer_Duplicate(t *testing.T) {
	t.Parallel()

	key := "duplicate_test_key_" + randomSuffix()
	tracing.RegisterReplacer(key, "value1")

	require.Panics(t, func() {
		tracing.RegisterReplacer(key, "value2")
	})
}

func TestExtractMetricsOpts_WithNodeName(t *testing.T) {
	t.Parallel()

	config := tracing.Config{
		Provider: tracing.None,
	}

	provider, err := tracing.NewProviderFromConfig(config)
	require.NoError(t, err)

	nodeNameProvider := tracing.NewProviderWithNodeName(provider, "test-node")
	tracer := nodeNameProvider.Tracer("test-tracer")

	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	require.NotNil(t, ctx)
	require.NotNil(t, span)
}

func randomSuffix() string {
	return "unique_" + os.Getenv("TEST_PROCESS_PID")
}
