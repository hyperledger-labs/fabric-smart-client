/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package scv2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContainerEnvVars(t *testing.T) {
	t.Parallel()

	base := containerConfig{ChannelName: "ch"}

	// collect the default key set so tests can reference it without hardcoding counts
	defaults := containerEnvVars(base)
	defaultCount := len(defaults)

	tests := []struct {
		name      string
		overrides map[string]string
		check     func(t *testing.T, result []string)
	}{
		{
			name:      "no overrides returns sorted defaults",
			overrides: nil,
			check: func(t *testing.T, result []string) {
				require.Equal(t, defaults, result, "result should be stable across calls")
				for i := 1; i < len(result); i++ {
					require.Less(t, result[i-1], result[i], "env vars must be sorted")
				}
			},
		},
		{
			name:      "non-empty value overrides a default",
			overrides: map[string]string{"SC_ORDERER_BLOCK_SIZE": "10"},
			check: func(t *testing.T, result []string) {
				require.Len(t, result, defaultCount)
				require.Contains(t, result, "SC_ORDERER_BLOCK_SIZE=10")
				require.NotContains(t, result, "SC_ORDERER_BLOCK_SIZE=1")
			},
		},
		{
			name:      "empty value removes a default",
			overrides: map[string]string{"SC_VC_LOGGING_LOGSPEC": ""},
			check: func(t *testing.T, result []string) {
				require.Len(t, result, defaultCount-1)
				for _, entry := range result {
					require.NotContains(t, entry, "SC_VC_LOGGING_LOGSPEC")
				}
			},
		},
		{
			name:      "non-empty value adds a new var",
			overrides: map[string]string{"SC_ORDERER_BLOCK_TIMEOUT": "1s"},
			check: func(t *testing.T, result []string) {
				require.Len(t, result, defaultCount+1)
				require.Contains(t, result, "SC_ORDERER_BLOCK_TIMEOUT=1s")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			cfg.EnvVarOverrides = tc.overrides
			tc.check(t, containerEnvVars(cfg))
		})
	}
}

func TestContainerImage(t *testing.T) {
	t.Parallel()

	t.Run("returns default image when empty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, committerImageName, containerImage(containerConfig{}))
	})

	t.Run("returns custom image when set", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "my-image:latest", containerImage(containerConfig{Image: "my-image:latest"}))
	})
}
