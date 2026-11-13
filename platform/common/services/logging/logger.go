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
	"strings"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/flogging/httpadmin"
	"go.uber.org/zap"
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

	// IsEnabledFor reports whether the logger writes lines at level, which logging.spec
	// sets for the logger's name; With keeps the name. Use it to skip building arguments
	// that are expensive to compute.
	IsEnabledFor(level Level) bool
	// With returns a logger that adds the given alternating keys and values to every line.
	With(keysAndValues ...any) Logger
	// Named returns a logger whose name is this logger's name followed by "." and name.
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

// callerSkip is the number of frames between zap's sugared logger and the caller of a
// Logger method: the method itself, then logArgs, logf or logw, then write.
const callerSkip = 3

type logger struct {
	// base is the zap logger without the caller skip; Named and With derive from it.
	base *zap.Logger
	s    *zap.SugaredLogger
	name string
	// levels holds the per-name levels of flogging. It is nil when base does not write
	// through flogging, as in tests, and the level check then asks the zap core.
	levels *flogging.LoggerLevels
}

func newLogger(zapLogger *zap.Logger) *logger {
	l := &logger{
		base: zapLogger,
		s:    zapLogger.WithOptions(zap.AddCallerSkip(callerSkip)).Sugar(),
		name: zapLogger.Name(),
	}
	if c, ok := zapLogger.Core().(*flogging.Core); ok {
		l.levels = c.Levels
	}
	return l
}

func (l *logger) Named(name string) Logger { return newLogger(l.base.Named(name)) }

func (l *logger) With(keysAndValues ...any) Logger {
	return newLogger(l.base.Sugar().With(keysAndValues...).Desugar())
}

func (l *logger) IsEnabledFor(level Level) bool { return l.enabled(zapLevel(level)) }

// enabled checks the level of this logger's name. The zap core of flogging checks only
// the lowest level of the whole spec, which is why the name is looked up here.
func (l *logger) enabled(level zapcore.Level) bool {
	if l.levels != nil {
		return l.levels.Level(l.name).Enabled(level)
	}
	return l.base.Core().Enabled(level)
}

func (l *logger) logArgs(level zapcore.Level, args []any) {
	if l.enabled(level) {
		// Like fmt.Sprintln, which puts spaces between all operands, unlike fmt.Sprint.
		l.write(noCtx, level, strings.TrimSuffix(fmt.Sprintln(args...), "\n"), nil)
	}
}

func (l *logger) logf(ctx context.Context, level zapcore.Level, format string, args []any) {
	if l.enabled(level) {
		msg := format
		if len(args) > 0 {
			msg = fmt.Sprintf(format, args...)
		}
		l.write(ctx, level, msg, nil)
	}
}

func (l *logger) logw(ctx context.Context, level zapcore.Level, msg string, keysAndValues []any) {
	if l.enabled(level) {
		l.write(ctx, level, msg, keysAndValues)
	}
}

// noCtx is the context of the methods without one: it carries no span and no values.
var noCtx = context.Background()

// write writes an enabled line, with the span event and the context fields of ctx.
func (l *logger) write(ctx context.Context, level zapcore.Level, msg string, keysAndValues []any) {
	l.spanEvent(ctx, level, msg)
	keysAndValues = appendContextLogFields(ctx, keysAndValues)
	l.s.Logw(level, msg, keysAndValues...)
}

func (l *logger) Debug(args ...any)                 { l.logArgs(zapcore.DebugLevel, args) }
func (l *logger) Debugf(format string, args ...any) { l.logf(noCtx, zapcore.DebugLevel, format, args) }
func (l *logger) Debugw(msg string, kv ...any)      { l.logw(noCtx, zapcore.DebugLevel, msg, kv) }
func (l *logger) DebugfContext(ctx context.Context, format string, args ...any) {
	l.logf(ctx, zapcore.DebugLevel, format, args)
}

func (l *logger) DebugwContext(ctx context.Context, msg string, kv ...any) {
	l.logw(ctx, zapcore.DebugLevel, msg, kv)
}

func (l *logger) Info(args ...any)                 { l.logArgs(zapcore.InfoLevel, args) }
func (l *logger) Infof(format string, args ...any) { l.logf(noCtx, zapcore.InfoLevel, format, args) }
func (l *logger) Infow(msg string, kv ...any)      { l.logw(noCtx, zapcore.InfoLevel, msg, kv) }
func (l *logger) InfofContext(ctx context.Context, format string, args ...any) {
	l.logf(ctx, zapcore.InfoLevel, format, args)
}

func (l *logger) InfowContext(ctx context.Context, msg string, kv ...any) {
	l.logw(ctx, zapcore.InfoLevel, msg, kv)
}

func (l *logger) Warn(args ...any)                 { l.logArgs(zapcore.WarnLevel, args) }
func (l *logger) Warnf(format string, args ...any) { l.logf(noCtx, zapcore.WarnLevel, format, args) }
func (l *logger) Warnw(msg string, kv ...any)      { l.logw(noCtx, zapcore.WarnLevel, msg, kv) }
func (l *logger) WarnfContext(ctx context.Context, format string, args ...any) {
	l.logf(ctx, zapcore.WarnLevel, format, args)
}

func (l *logger) WarnwContext(ctx context.Context, msg string, kv ...any) {
	l.logw(ctx, zapcore.WarnLevel, msg, kv)
}

func (l *logger) Error(args ...any)                 { l.logArgs(zapcore.ErrorLevel, args) }
func (l *logger) Errorf(format string, args ...any) { l.logf(noCtx, zapcore.ErrorLevel, format, args) }
func (l *logger) Errorw(msg string, kv ...any)      { l.logw(noCtx, zapcore.ErrorLevel, msg, kv) }
func (l *logger) ErrorfContext(ctx context.Context, format string, args ...any) {
	l.logf(ctx, zapcore.ErrorLevel, format, args)
}

func (l *logger) ErrorwContext(ctx context.Context, msg string, kv ...any) {
	l.logw(ctx, zapcore.ErrorLevel, msg, kv)
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
	return newLogger(flogging.Global.ZapLogger(name)), nil
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
