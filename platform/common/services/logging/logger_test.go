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
	assert.Equal(t, "fsc", getReplacers()["github.com.hyperledger-labs.fabric-smart-client"],
		"the FSC package path is shortened by default")
}

// TestRegisterReplacer covers registration and the panic on a duplicate key.
func TestRegisterReplacer(t *testing.T) { //nolint:paralleltest // mutates the shared global replacer registry
	restoreReplacers(t)

	RegisterReplacer("some.long.package.path", "short")
	assert.Equal(t, "short", getReplacers()["some.long.package.path"])

	assert.PanicsWithValue(t, "replacer already exists", func() {
		RegisterReplacer("some.long.package.path", "other")
	}, "a key cannot be registered twice")

	assert.Equal(t, "short", getReplacers()["some.long.package.path"], "the first registration stands")
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

// TestGetPackageName_UnexpectedCallDepth checks a frame beyond the stack is reported
// rather than panicked on.
func TestGetPackageName_UnexpectedCallDepth(t *testing.T) {
	t.Parallel()

	name, err := getPackageName(1000)

	require.Error(t, err)
	assert.Empty(t, name)
}

func depth1() (string, error) { return depth2() }
func depth2() (string, error) { return getPackageName(2) }

type pkgNameHolder struct{}

func (pkgNameHolder) method() (string, error) { return depth2() }

// TestGetPackageName checks getPackageName resolves the package of a function and of a
// method.
func TestGetPackageName(t *testing.T) {
	t.Parallel()
	const pkg = "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	for name, call := range map[string]func() (string, error){
		"function": depth1,
		"method":   pkgNameHolder{}.method,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := call()
			require.NoError(t, err)
			assert.Equal(t, pkg, got)
		})
	}
}

// wrapperGetLogger and wrapperMustGetLogger are a wrapper pair as GetLoggerWithReplacements
// expects it, with a replacement that marks the name.
func wrapperGetLogger() (Logger, error) {
	return GetLoggerWithReplacements(map[string]string{"logging": "wrapped"}, nil)
}

func wrapperMustGetLogger() Logger {
	l, err := wrapperGetLogger()
	if err != nil {
		panic(err)
	}
	return l
}

// TestGetLoggerEntryPoints checks each entry point names the logger after the package it
// documents: the direct caller for GetLogger, the caller of the wrapper pair for
// GetLoggerWithReplacements.
func TestGetLoggerEntryPoints(t *testing.T) { //nolint:paralleltest // reads the shared global replacer registry
	l, err := GetLogger("p")
	require.NoError(t, err)
	assert.Equal(t, "fsc.platform.common.services.logging.p", l.(*logger).name)

	assert.Equal(t, "github.com.hyperledger-labs.fabric-smart-client.platform.common.services.wrapped",
		wrapperMustGetLogger().(*logger).name, "only the given replacements apply")
}

// TestMustGetLogger checks the happy path. MustGetLogger adds a frame of its own, so from a
// test runtime.Caller(4) lands on the calling test function rather than testing.tRunner, and
// the name resolves -- with the registered replacements applied.
func TestMustGetLogger(t *testing.T) { //nolint:paralleltest // reads the shared global replacer registry
	l := MustGetLogger()

	require.NotNil(t, l)
	assert.Equal(t, "fsc.platform.common.services.logging", l.(*logger).name,
		"the package path is resolved and the FSC replacement applied")
}

// TestMustGetLoggerParams checks params are appended to the resolved name.
func TestMustGetLoggerParams(t *testing.T) { //nolint:paralleltest // reads the shared global replacer registry
	assert.Equal(t, "fsc.platform.common.services.logging.component",
		MustGetLogger("component").(*logger).name)
}
