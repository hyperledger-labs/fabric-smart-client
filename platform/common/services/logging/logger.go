/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/flogging/httpadmin"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Logger writes log lines under a logger name.
//
// Each level has five methods: an unformatted one, which joins its arguments with spaces;
// a printf-style one (suffix f); a structured one (suffix w), which takes a message and
// alternating keys and values; and context-aware forms of the last two. A context-aware
// method also adds the context fields registered with [RegisterContextLogField], and, when
// ctx carries a recording span, records the message as an event on that span and sets
// the span status to Error at [ErrorLevel].
//
// A method whose level is disabled for the logger returns without formatting its
// arguments. Prefer the structured methods for values that tools should read, and the
// context-aware ones whenever a context is at hand.
//
// To add fields or a name segment, use With or Named rather than wrapping a Logger: a
// line reports the caller of the Logger method, which for a wrapper is the wrapper.
// Logger is implemented by FSC, and methods may be added to it; embed [MockLogger] to
// write a test double.
type Logger interface {
	Debug(args ...any)
	Debugf(format string, args ...any)
	Debugw(msg string, keysAndValues ...any)
	DebugfContext(ctx context.Context, format string, args ...any)
	DebugwContext(ctx context.Context, msg string, keysAndValues ...any)

	Info(args ...any)
	Infof(format string, args ...any)
	Infow(msg string, keysAndValues ...any)
	InfofContext(ctx context.Context, format string, args ...any)
	InfowContext(ctx context.Context, msg string, keysAndValues ...any)

	Warn(args ...any)
	Warnf(format string, args ...any)
	Warnw(msg string, keysAndValues ...any)
	WarnfContext(ctx context.Context, format string, args ...any)
	WarnwContext(ctx context.Context, msg string, keysAndValues ...any)

	Error(args ...any)
	Errorf(format string, args ...any)
	Errorw(msg string, keysAndValues ...any)
	ErrorfContext(ctx context.Context, format string, args ...any)
	ErrorwContext(ctx context.Context, msg string, keysAndValues ...any)

	// IsEnabledFor reports whether the logger writes lines at level: logging.spec
	// enables level for the logger's name, which With keeps, and the handler is enabled
	// for it (see [SetHandler]). Use it to skip building arguments that are expensive to
	// compute.
	IsEnabledFor(level Level) bool
	// With returns a logger that adds the given alternating keys and values to every line.
	With(keysAndValues ...any) Logger
	// Named returns a logger whose name is this logger's name followed by "." and name,
	// or this logger if name is empty.
	Named(name string) Logger
}

// Level is the severity of a log line. Its values are those of [slog.Level].
type Level = slog.Level

// The levels a [Logger] writes at.
const (
	DebugLevel = slog.LevelDebug
	InfoLevel  = slog.LevelInfo
	WarnLevel  = slog.LevelWarn
	ErrorLevel = slog.LevelError
)

// zapLevel maps level to the highest zap level not above it, so a level between two of
// the constants above is enabled only when the lower one is. Levels below DebugLevel map
// to DebugLevel.
func zapLevel(level Level) zapcore.Level {
	switch {
	case level < InfoLevel:
		return zapcore.DebugLevel
	case level < WarnLevel:
		return zapcore.InfoLevel
	case level < ErrorLevel:
		return zapcore.WarnLevel
	default:
		return zapcore.ErrorLevel
	}
}

// callerSkip is the number of frames runtime.Callers skips in write to reach the caller
// of a Logger method: runtime.Callers, write, then logArgs, logf or logw, then the method.
const callerSkip = 4

// logger is the front end of every FSC logger: it checks the level, builds a slog.Record
// and hands it to its handler, derived from the backend with the logger's name and With
// attributes.
type logger struct {
	name  string
	attrs []slog.Attr
	// levels holds the per-name levels of logging.spec. It is nil for a logger with a
	// fixed backend, as in tests, which then leaves the level check to the handler.
	levels  *flogging.LoggerLevels
	handler *handlerCache
}

// newLogger returns a logger named name with attrs. A nil root writes through the
// current backend and applies logging.spec.
func newLogger(name string, attrs []slog.Attr, root *backend) *logger {
	l := &logger{name: name, attrs: attrs}
	if root == nil {
		l.levels = flogging.Global.LoggerLevels
	}
	l.handler = &handlerCache{root: root, derive: func(h slog.Handler) slog.Handler {
		return h.WithAttrs(append([]slog.Attr{slog.String(loggerAttr, l.name)}, l.attrs...))
	}}
	return l
}

// newZapLogger returns a logger named name that writes to core through zapslog, with no
// logging.spec check, for tests.
func newZapLogger(core zapcore.Core, name string) *logger {
	return newLogger(name, nil, &backend{h: newZapHandler(core)})
}

func (l *logger) Named(name string) Logger {
	if name == "" {
		return l
	}
	if l.name != "" {
		name = l.name + "." + name
	}
	return newLogger(name, l.attrs, l.handler.root)
}

func (l *logger) With(keysAndValues ...any) Logger {
	// slog.Group turns alternating keys and values into attributes as Record.Add does.
	attrs := slices.Concat(l.attrs, slog.Group("", keysAndValues...).Value.Group())
	return newLogger(l.name, attrs, l.handler.root)
}

func (l *logger) IsEnabledFor(level Level) bool {
	_, ok := l.enabled(noCtx, level)
	return ok
}

// enabled returns the logger's handler and whether a line at level is written: when
// logging.spec enables level for the logger's name and the handler is enabled for it.
// The spec is checked first, so a disabled line costs only that lookup.
func (l *logger) enabled(ctx context.Context, level Level) (slog.Handler, bool) {
	if !specEnabled(l.levels, l.name, level) {
		return nil, false
	}
	h := l.handler.get()
	return h, h.Enabled(ctx, level)
}

func (l *logger) logArgs(level Level, args []any) {
	if h, ok := l.enabled(noCtx, level); ok {
		// Like fmt.Sprintln, which puts spaces between all operands, unlike fmt.Sprint.
		l.write(noCtx, h, level, strings.TrimSuffix(fmt.Sprintln(args...), "\n"), nil)
	}
}

func (l *logger) logf(ctx context.Context, level Level, format string, args []any) { //nolint:contextcheck // replaces only a nil ctx
	if ctx == nil {
		ctx = noCtx
	}
	if h, ok := l.enabled(ctx, level); ok {
		msg := format
		if len(args) > 0 {
			msg = fmt.Sprintf(format, args...)
		}
		l.write(ctx, h, level, msg, nil)
	}
}

func (l *logger) logw(ctx context.Context, level Level, msg string, keysAndValues []any) { //nolint:contextcheck // replaces only a nil ctx
	if ctx == nil {
		ctx = noCtx
	}
	if h, ok := l.enabled(ctx, level); ok {
		l.write(ctx, h, level, msg, keysAndValues)
	}
}

// noCtx is the context of the methods without one: it carries no span and no values.
var noCtx = context.Background()

// write writes an enabled line to h, with the span event and the context fields of ctx.
func (l *logger) write(ctx context.Context, h slog.Handler, level Level, msg string, keysAndValues []any) {
	l.spanEvent(ctx, level, msg)
	var pcs [1]uintptr
	runtime.Callers(callerSkip, pcs[:])
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(keysAndValues...)
	addContextLogFields(ctx, &r)
	// A handler reports a failed write, which a log call has no one to return to.
	_ = h.Handle(ctx, r)
}

func (l *logger) Debug(args ...any)                 { l.logArgs(DebugLevel, args) }
func (l *logger) Debugf(format string, args ...any) { l.logf(noCtx, DebugLevel, format, args) }
func (l *logger) Debugw(msg string, kv ...any)      { l.logw(noCtx, DebugLevel, msg, kv) }
func (l *logger) DebugfContext(ctx context.Context, format string, args ...any) {
	l.logf(ctx, DebugLevel, format, args)
}

func (l *logger) DebugwContext(ctx context.Context, msg string, kv ...any) {
	l.logw(ctx, DebugLevel, msg, kv)
}

func (l *logger) Info(args ...any)                 { l.logArgs(InfoLevel, args) }
func (l *logger) Infof(format string, args ...any) { l.logf(noCtx, InfoLevel, format, args) }
func (l *logger) Infow(msg string, kv ...any)      { l.logw(noCtx, InfoLevel, msg, kv) }
func (l *logger) InfofContext(ctx context.Context, format string, args ...any) {
	l.logf(ctx, InfoLevel, format, args)
}

func (l *logger) InfowContext(ctx context.Context, msg string, kv ...any) {
	l.logw(ctx, InfoLevel, msg, kv)
}

func (l *logger) Warn(args ...any)                 { l.logArgs(WarnLevel, args) }
func (l *logger) Warnf(format string, args ...any) { l.logf(noCtx, WarnLevel, format, args) }
func (l *logger) Warnw(msg string, kv ...any)      { l.logw(noCtx, WarnLevel, msg, kv) }
func (l *logger) WarnfContext(ctx context.Context, format string, args ...any) {
	l.logf(ctx, WarnLevel, format, args)
}

func (l *logger) WarnwContext(ctx context.Context, msg string, kv ...any) {
	l.logw(ctx, WarnLevel, msg, kv)
}

func (l *logger) Error(args ...any)                 { l.logArgs(ErrorLevel, args) }
func (l *logger) Errorf(format string, args ...any) { l.logf(noCtx, ErrorLevel, format, args) }
func (l *logger) Errorw(msg string, kv ...any)      { l.logw(noCtx, ErrorLevel, msg, kv) }
func (l *logger) ErrorfContext(ctx context.Context, format string, args ...any) {
	l.logf(ctx, ErrorLevel, format, args)
}

func (l *logger) ErrorwContext(ctx context.Context, msg string, kv ...any) {
	l.logw(ctx, ErrorLevel, msg, kv)
}

// MustGetLogger is [GetLogger] for package-level loggers, var logger =
// logging.MustGetLogger(), where there is no caller to return an error to. It panics
// if the caller's package cannot be determined, which is a programming error.
func MustGetLogger(params ...string) Logger {
	l, err := newNamedLogger(getReplacers(), params, callerOfEntryPoint)
	if err != nil {
		panic(err)
	}
	return l
}

// GetLogger returns a Logger named after the package of its caller, with the registered
// replacers (see [RegisterReplacer]) applied and params appended to the name. It fails
// only if the caller's package cannot be determined.
func GetLogger(params ...string) (Logger, error) {
	return newNamedLogger(getReplacers(), params, callerOfEntryPoint)
}

// GetLoggerWithReplacements is for a package that wraps the pair MustGetLogger and
// GetLogger with replacements of its own: its MustGetLogger calls its GetLogger, which
// calls GetLoggerWithReplacements. The logger is named after the package of the caller
// of that MustGetLogger, which is three frames above GetLoggerWithReplacements.
func GetLoggerWithReplacements(replacements map[string]string, params []string) (Logger, error) {
	return newNamedLogger(replacements, params, callerOfEntryPoint+2)
}

// callerOfEntryPoint is the frame, counted from getPackageName, of the caller of the
// exported function that calls newNamedLogger: getPackageName, newNamedLogger, that function, and
// its caller.
const callerOfEntryPoint = 3

func newNamedLogger(replacements map[string]string, params []string, skip int) (Logger, error) {
	fullPkgName, err := getPackageName(skip)
	if err != nil {
		return nil, err
	}
	name := loggerName(fullPkgName, replacements, params...)
	return newLogger(name, nil, nil), nil
}

// getPackageName returns the package path of the function skip frames above it.
func getPackageName(skip int) (string, error) {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		return "", errors.New("failed to get caller package name")
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "", errors.New("failed to get caller package name")
	}
	fullFuncName := fn.Name()
	lastSlash := strings.LastIndex(fullFuncName, "/")
	if lastSlash < 0 {
		return "", errors.Errorf("caller package name has no path separator: %s", fullFuncName)
	}
	dotAfterSlash := strings.Index(fullFuncName[lastSlash:], ".")
	if dotAfterSlash < 0 {
		return "", errors.Errorf("caller package name has no function separator: %s", fullFuncName)
	}

	return fullFuncName[:lastSlash+dotAfterSlash], nil
}

// NewSpecHandler returns the HTTP handler that reads and changes the logging spec of
// the running process, served by the operations server at /logspec.
func NewSpecHandler() http.Handler {
	return httpadmin.NewSpecHandler()
}

func loggerName(fullPkgName string, replacements map[string]string, params ...string) string {
	nameParts := append(strings.Split(fullPkgName, "/"), params...)
	name := strings.Join(nameParts, ".")

	for old, newVal := range replacements {
		name = strings.ReplaceAll(name, old, newVal)
	}
	return name
}

// getReplacers returns a copy of the registered replacers, so the caller can range over
// it without holding the lock.
func getReplacers() map[string]string {
	replacersMutex.RLock()
	defer replacersMutex.RUnlock()
	return maps.Clone(replacers)
}
