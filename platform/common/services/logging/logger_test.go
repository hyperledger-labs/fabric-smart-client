/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restoreReplacers snapshots the global replacer registry and puts it back afterwards.
// RegisterReplacer panics on a duplicate key, so a leaked entry would break every later
// test in the package.
func restoreReplacers(t *testing.T) {
	t.Helper()

	replacersMutex.RLock()
	saved := maps.Clone(replacers)
	replacersMutex.RUnlock()

	t.Cleanup(func() {
		replacersMutex.Lock()
		replacers = saved
		replacersMutex.Unlock()
	})
}

// TestReplacers checks the registry starts with the FSC package-path shortening.
func TestReplacers(t *testing.T) { //nolint:paralleltest // reads the shared global replacer registry
	assert.Equal(t, "fsc", Replacers()["github.com.hyperledger-labs.fabric-smart-client"],
		"the FSC package path is shortened by default")
}

// TestRegisterReplacer covers registration and the panic on a duplicate key.
func TestRegisterReplacer(t *testing.T) { //nolint:paralleltest // mutates the shared global replacer registry
	restoreReplacers(t)

	RegisterReplacer("some.long.package.path", "short")
	assert.Equal(t, "short", Replacers()["some.long.package.path"])

	assert.PanicsWithValue(t, "replacer already exists", func() {
		RegisterReplacer("some.long.package.path", "other")
	}, "a key cannot be registered twice")

	assert.Equal(t, "short", Replacers()["some.long.package.path"], "the first registration stands")
}

// TestLoggerName covers the name construction: the package path is dotted, any params are
// appended, and every replacement in force is applied.
func TestLoggerName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fullPkgName  string
		replacements map[string]string
		params       []string
		expected     string
	}{
		{
			name:        "package path is dotted",
			fullPkgName: "github.com/org/repo/pkg",
			expected:    "github.com.org.repo.pkg",
		},
		{
			name:        "params are appended",
			fullPkgName: "github.com/org/repo/pkg",
			params:      []string{"sub", "component"},
			expected:    "github.com.org.repo.pkg.sub.component",
		},
		{
			name:         "replacements are applied",
			fullPkgName:  "github.com/org/repo/pkg",
			replacements: map[string]string{"github.com.org.repo": "repo"},
			expected:     "repo.pkg",
		},
		{
			name:         "a replacement that does not match leaves the name alone",
			fullPkgName:  "github.com/org/repo/pkg",
			replacements: map[string]string{"not.present": "x"},
			expected:     "github.com.org.repo.pkg",
		},
		{
			name:        "empty package name",
			fullPkgName: "",
			expected:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, loggerName(tc.fullPkgName, tc.replacements, tc.params...))
		})
	}
}

// TestNewSpecHandler checks a handler is returned for the logspec endpoint.
func TestNewSpecHandler(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, NewSpecHandler())
}

// TestGetPackageName_UnexpectedCallDepth checks a caller at the wrong depth is reported
// rather than panicked on. GetPackageName reads runtime.Caller(4), which assumes the
// production call chain; called directly from a test there is no fourth frame.
func TestGetPackageName_UnexpectedCallDepth(t *testing.T) {
	t.Parallel()

	name, err := GetPackageName()

	require.Error(t, err)
	assert.Empty(t, name)
}

// TestGetLogger_ReportsUnexpectedCallDepth checks the error is propagated rather than
// panicking, for each of the three entry points that reach GetPackageName.
func TestGetLogger_ReportsUnexpectedCallDepth(t *testing.T) {
	t.Parallel()

	t.Run("GetLogger", func(t *testing.T) {
		t.Parallel()

		logger, err := GetLogger()

		require.Error(t, err)
		assert.Nil(t, logger)
	})

	t.Run("GetLoggerWithReplacements", func(t *testing.T) {
		t.Parallel()

		logger, err := GetLoggerWithReplacements(map[string]string{"a": "b"}, nil)

		require.Error(t, err)
		assert.Nil(t, logger)
	})
}

// TestMustGetLogger checks the happy path. MustGetLogger adds a frame of its own, so from a
// test runtime.Caller(4) lands on the calling test function rather than testing.tRunner, and
// the name resolves -- with the registered replacements applied.
func TestMustGetLogger(t *testing.T) { //nolint:paralleltest // reads the shared global replacer registry
	logger := MustGetLogger()

	require.NotNil(t, logger)
	assert.Equal(t, "fsc.platform.common.services.logging", logger.Zap().Name(),
		"the package path is resolved and the FSC replacement applied")
}

// TestMustGetLoggerParams checks params are appended to the resolved name.
func TestMustGetLoggerParams(t *testing.T) { //nolint:paralleltest // reads the shared global replacer registry
	assert.Equal(t, "fsc.platform.common.services.logging.component",
		MustGetLogger("component").Zap().Name())
}
