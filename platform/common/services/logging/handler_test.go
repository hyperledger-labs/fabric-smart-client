/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedBuffer is a bytes.Buffer safe for concurrent writes.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

// lines decodes the JSON lines written so far.
func (b *lockedBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for l := range strings.Lines(b.b.String()) {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m))
		out = append(out, m)
	}
	return out
}

// appHandler sets a JSON handler with the source of each line, as an application would,
// and Init with spec and the default handler's writer at out. Both are restored at the
// end of the test.
func appHandler(t *testing.T, spec string, level slog.Level) (app, out *lockedBuffer) {
	t.Helper()
	app, out = &lockedBuffer{}, &lockedBuffer{}
	initForTest(t, Config{Format: "json", LogSpec: spec, Writer: out})
	t.Cleanup(func() { SetHandler(nil) })
	SetHandler(slog.NewJSONHandler(app, &slog.HandlerOptions{AddSource: true, Level: level}))
	return app, out
}

// pkgLogger is created before any test calls SetHandler, like a package-level logger.
var pkgLogger = MustGetLogger("pkg")

func TestSetHandler(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	app, out := appHandler(t, "info", slog.LevelDebug)

	pkgLogger.Infow("to app", "k", "v")
	pkgLogger.Debugf("dropped by logging.spec")
	SetHandler(nil)
	pkgLogger.Infof("to default")

	lines := app.lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "to app", lines[0]["msg"])
	assert.Equal(t, "v", lines[0]["k"])
	assert.Equal(t, "fsc.platform.common.services.logging.pkg", lines[0][loggerAttr])
	defaultLines := out.lines(t)
	require.NotEmpty(t, defaultLines)
	assert.Equal(t, "to default", defaultLines[len(defaultLines)-1]["msg"])
}

// TestSetHandler_BothFilters checks a line reaches the handler only if both logging.spec
// and the handler enable it, and that a spec change applies to a running logger.
func TestSetHandler_BothFilters(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	app, _ := appHandler(t, "debug", slog.LevelInfo)
	l := MustGetLogger()

	l.Debugf("dropped by the handler")
	require.False(t, l.IsEnabledFor(DebugLevel))
	l.Infof("written")
	require.NoError(t, flogging.Global.ActivateSpec("warn"))
	l.Infof("dropped by logging.spec")

	lines := app.lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "written", lines[0]["msg"])
}

// TestSetHandler_NamedAndWith checks a derived logger passes its full name once and its
// With fields, also after SetHandler changes the handler under it.
func TestSetHandler_NamedAndWith(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	l := MustGetLogger().Named("a").With("k1", 1).Named("b").With("k2", "v")
	l.Infof("derive from the default handler first")
	app, _ := appHandler(t, "info", slog.LevelDebug)

	l.Infof("x")

	lines := app.lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "fsc.platform.common.services.logging.a.b", lines[0][loggerAttr])
	assert.InDelta(t, 1, lines[0]["k1"], 0)
	assert.Equal(t, "v", lines[0]["k2"])
	// JSON with a repeated key decodes to its last value; count the key in the raw line.
	assert.Equal(t, 1, strings.Count(app.b.String(), `"logger":`))
}

// TestSetHandler_Caller checks every method reports its caller to an application handler,
// for package, derived and test loggers.
func TestSetHandler_Caller(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	app, _ := appHandler(t, "debug", slog.LevelDebug)
	base := MustGetLogger()
	calls := 0
	for _, l := range []Logger{base, base.Named("child"), base.With("k", "v")} {
		calls += callEveryMethod(t.Context(), l)
	}

	lines := app.lines(t)
	require.Len(t, lines, calls)
	for _, l := range lines {
		assert.True(t, strings.HasSuffix(l["source"].(map[string]any)["function"].(string), ".callEveryMethod"), l)
	}
}

// TestSetHandler_Context checks an application handler gets the context fields, and the
// span gets its event and status, as with the default handler.
func TestSetHandler_Context(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	t.Cleanup(resetContextLogFields)
	app, _ := appHandler(t, "info", slog.LevelDebug)
	key := ctxKey{"app"}
	RegisterContextLogField("req", key)
	ctx, ended := recordingSpanCtx(t)

	MustGetLogger().ErrorfContext(context.WithValue(ctx, key, "r1"), "failed")

	lines := app.lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "r1", lines[0]["req"])
	span := ended()
	require.Len(t, span.Events, 1)
	assert.Equal(t, "failed", span.Events[0].Name)
	assert.Equal(t, "failed", span.Status.Description)
}

// TestSetHandler_DisabledDoesntFormat checks a line the handler drops is neither
// formatted nor recorded as a span event.
func TestSetHandler_DisabledDoesntFormat(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	appHandler(t, "debug", slog.LevelInfo)
	ctx, ended := recordingSpanCtx(t)
	n := 0

	MustGetLogger().DebugfContext(ctx, "%s", countingStringer{&n})

	assert.Zero(t, n)
	assert.Empty(t, ended().Events)
}

// TestSetHandler_Concurrent runs SetHandler and Init while other goroutines log, for the
// race detector.
func TestSetHandler_Concurrent(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	app, _ := appHandler(t, "info", slog.LevelDebug)
	h := slog.NewJSONHandler(app, nil)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				pkgLogger.Infof("x")
			}
		})
	}
	for range 100 {
		SetHandler(nil)
		SetHandler(h)
		require.NoError(t, Init(Config{Format: "json", LogSpec: "info"}))
	}
	wg.Wait()
}

// TestHandler checks slog lines through Handler(name) follow logging.spec under name and
// reach the current handler with their attributes and groups, also after SetHandler.
func TestHandler(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	l := slog.New(Handler("app")).With("k", "v").WithGroup("g")
	l.Info("derive from the default handler first")
	app, _ := appHandler(t, "app=warn:info", slog.LevelDebug)

	l.Info("dropped by logging.spec", "a", 1)
	l.Warn("written", "a", 1)

	lines := app.lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "written", lines[0]["msg"])
	assert.Equal(t, "app", lines[0][loggerAttr])
	assert.Equal(t, "v", lines[0]["k"])
	assert.Equal(t, map[string]any{"a": float64(1)}, lines[0]["g"])
}

func TestHandler_Default(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	var buf bytes.Buffer
	initForTest(t, Config{Format: "[%{module}] %{shortfunc} %{message}", LogSpec: "info", Writer: &buf})
	buf.Reset() // drop what Init itself logs

	slog.New(Handler("app")).Info("hello", "k", "v")

	assert.Equal(t, "[app] TestHandler_Default hello k=v\n", buf.String())
}
