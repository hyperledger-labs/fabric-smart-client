/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func TestNewProviderFromConfig_None(t *testing.T) {
	t.Parallel()

	p, err := tracing.NewProviderFromConfig(tracing.Config{Provider: tracing.None})
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestNewProviderFromConfig_Default(t *testing.T) {
	t.Parallel()

	p, err := tracing.NewProviderFromConfig(tracing.Config{Provider: "unknown"})
	require.NoError(t, err)
	require.NotNil(t, p)
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
