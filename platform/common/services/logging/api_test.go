/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// initForTest applies c and restores the default configuration, writing to stderr, when
// the test ends.
func initForTest(t *testing.T, c Config) {
	t.Helper()
	t.Cleanup(func() { require.NoError(t, Init(Config{Writer: os.Stderr})) })
	require.NoError(t, Init(c))
}

// TestIsEnabledFor checks the level of the logger's own name decides, for loggers created
// before and after Init and for derived ones, which a spec matches by name prefix. With a
// spec that enables Debug for another logger only, the zap core of flogging alone would
// report Debug enabled for every logger.
func TestIsEnabledFor(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	before := MustGetLogger("before")
	for _, tc := range []struct {
		spec  string
		debug bool
	}{
		{"debug", true},
		{"info", false},
		{"fsc.some.other.logger=debug:info", false},
		{"fsc.platform.common.services.logging.before,fsc.platform.common.services.logging.after=debug:info", true},
	} {
		initForTest(t, Config{LogSpec: tc.spec})
		after := MustGetLogger("after")
		for name, l := range map[string]Logger{"before": before, "after": after, "derived": before.Named("x").With("k", "v")} {
			assert.Equal(t, tc.debug, l.IsEnabledFor(DebugLevel), "%s logger with spec %q", name, tc.spec)
			assert.True(t, l.IsEnabledFor(InfoLevel), "%s logger with spec %q", name, tc.spec)
		}
	}
}

// countingStringer counts how often it is formatted.
type countingStringer struct{ n *int }

func (c countingStringer) String() string { *c.n++; return "counted" }

// TestDisabledMethodsDontFormat checks no method of a disabled level formats its arguments.
func TestDisabledMethodsDontFormat(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	initForTest(t, Config{LogSpec: "fsc.some.other.logger=debug:error"})
	l := MustGetLogger()
	n := 0
	s := countingStringer{&n}
	ctx := context.Background()

	l.Debug(s)
	l.Debugf("%s", s)
	l.Debugw("msg", "k", s)
	l.DebugfContext(ctx, "%s", s)
	l.DebugwContext(ctx, "msg", "k", s)
	l.Info(s)
	l.Infof("%s", s)
	l.Infow("msg", "k", s)
	l.Warn(s)
	l.Warnf("%s", s)
	l.Warnw("msg", "k", s)

	assert.Zero(t, n)
}

// TestUnformattedMethods checks the unformatted methods put spaces between all their
// TestDisabledAllocs checks a disabled Debug or Debugf allocates only the slice of its
// arguments, whatever the handler.
func TestDisabledAllocs(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	initForTest(t, Config{LogSpec: "info"})
	l := MustGetLogger()
	assert.InDelta(t, 1, testing.AllocsPerRun(1000, func() { l.Debug("x", "k", "v") }), 0.01)
	assert.InDelta(t, 1, testing.AllocsPerRun(1000, func() { l.Debugf("x %s", "v") }), 0.01)
}

// arguments, as before; fmt.Sprint would join two strings without one.
func TestUnformattedMethods(t *testing.T) {
	t.Parallel()
	l, r := NewTestLogger(t)
	l.Info("a", "b", 1)
	assert.Equal(t, []string{"a b 1"}, r.Messages())
}

// TestStructuredMethods checks every structured method writes its message and its
// key/value pairs as fields, at its level.
func TestStructuredMethods(t *testing.T) {
	t.Parallel()
	core, observed := observer.New(zapcore.DebugLevel)
	l := newZapLogger(core, "")
	ctx := t.Context()

	l.Debugw("m", "k", "v")
	l.DebugwContext(ctx, "m", "k", "v")
	l.Infow("m", "k", "v")
	l.InfowContext(ctx, "m", "k", "v")
	l.Warnw("m", "k", "v")
	l.WarnwContext(ctx, "m", "k", "v")
	l.Errorw("m", "k", "v")
	l.ErrorwContext(ctx, "m", "k", "v")

	entries := observed.All()
	require.Len(t, entries, 8)
	levels := []zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel, zapcore.WarnLevel, zapcore.ErrorLevel}
	for i, e := range entries {
		assert.Equal(t, levels[i/2], e.Level)
		assert.Equal(t, "m", e.Message)
		assert.Equal(t, map[string]any{"k": "v"}, e.ContextMap())
	}
}

// TestCallerOfEveryMethod checks every method reports the function that called it, for
// loggers from MustGetLogger, Named, With and NewTestLogger. %{shortfunc} prints the
// caller's function name.
func TestCallerOfEveryMethod(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	var buf bytes.Buffer
	initForTest(t, Config{Format: "%{shortfunc}", LogSpec: "debug", Writer: &buf})
	buf.Reset() // drop what Init itself logs
	base := MustGetLogger()
	testLogger, r := NewTestLogger(t)
	loggers := []Logger{base, base.Named("child"), base.With("k", "v"), testLogger, testLogger.Named("c").With("k", "v")}

	calls := 0
	for _, l := range loggers {
		calls += callEveryMethod(t.Context(), l)
	}

	got := strings.Fields(buf.String()) // one function name per line, then any With fields
	got = slices.DeleteFunc(got, func(f string) bool { return f == "k=v" })
	for _, e := range r.Entries() {
		// The recorder's format is "[name] shortfunc -> LEVEL id message".
		got = append(got, strings.Fields(e)[1])
	}
	require.Len(t, got, calls)
	for _, f := range got {
		require.Equal(t, "callEveryMethod", f)
	}
}

// callEveryMethod calls every level method of l once and returns the number of calls.
func callEveryMethod(ctx context.Context, l Logger) int {
	l.Debug("x")
	l.Debugf("x")
	l.Debugw("x")
	l.DebugfContext(ctx, "x")
	l.DebugwContext(ctx, "x")
	l.Info("x")
	l.Infof("x")
	l.Infow("x")
	l.InfofContext(ctx, "x")
	l.InfowContext(ctx, "x")
	l.Warn("x")
	l.Warnf("x")
	l.Warnw("x")
	l.WarnfContext(ctx, "x")
	l.WarnwContext(ctx, "x")
	l.Error("x")
	l.Errorf("x")
	l.Errorw("x")
	l.ErrorfContext(ctx, "x")
	l.ErrorwContext(ctx, "x")
	return 20
}

// TestInitInvalidConfig checks Init rejects an invalid spec or format, names the key, and
// leaves the configuration as it was.
func TestInitInvalidConfig(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	initForTest(t, Config{LogSpec: "warn"})

	err := Init(Config{LogSpec: "debug:nope=bogus"})
	require.ErrorContains(t, err, "invalid logging.spec")

	err = Init(Config{LogSpec: "debug", Format: "%{color:blink}"})
	require.ErrorContains(t, err, "invalid logging.format")

	err = Init(Config{LogSpec: "bogus", Format: "json"})
	require.ErrorContains(t, err, "invalid logging.spec")

	t.Setenv("FABRIC_LOGGING_SPEC", "debug:nope=bogus")
	err = Init(Config{})
	require.ErrorContains(t, err, "invalid FABRIC_LOGGING_SPEC", "an empty LogSpec reads the env var, and the error says so")

	assert.Equal(t, "warn", flogging.Global.Spec(), "the spec is unchanged")
	var buf bytes.Buffer
	require.NoError(t, Init(Config{LogSpec: "warn", Writer: &buf}))
	MustGetLogger().Warn("hello")
	assert.False(t, strings.HasPrefix(buf.String(), "{"), "the format is still the pattern, not json: %s", buf.String())
}

// TestInitNilWriterKeepsWriter checks a later Init without a writer keeps the earlier one.
func TestInitNilWriterKeepsWriter(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	var buf bytes.Buffer
	initForTest(t, Config{Writer: &buf})
	require.NoError(t, Init(Config{LogSpec: "info"}))

	MustGetLogger().Info("kept")

	assert.Contains(t, buf.String(), "kept")
}

// TestLogspecChangesRunningLogger checks a PUT /logspec changes the level of an existing
// logger at once, and of that logger only.
func TestLogspecChangesRunningLogger(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	initForTest(t, Config{LogSpec: "info"})
	l := MustGetLogger("running")
	require.False(t, l.IsEnabledFor(DebugLevel))

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"spec":"fsc.platform.common.services.logging.running=debug:info"}`)
	NewSpecHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/logspec", body))

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.True(t, l.IsEnabledFor(DebugLevel))
	assert.False(t, MustGetLogger("other").IsEnabledFor(DebugLevel))
}

// TestMockLoggerKeyValues checks the structured methods of MockLogger print k=v pairs.
func TestMockLoggerKeyValues(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a=1 b=x c", keyValues([]any{"a", 1, "b", "x", "c"}))
}
