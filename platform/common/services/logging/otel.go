/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"

	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	loggerNameKey = "logger.name"
)

type otelLogger interface {
	DebugfContext(ctx context.Context, template string, args ...any)
	DebugwContext(ctx context.Context, template string, args ...any)
	InfofContext(ctx context.Context, template string, args ...any)
	InfowContext(ctx context.Context, template string, args ...any)
	WarnfContext(ctx context.Context, template string, args ...any)
	WarnwContext(ctx context.Context, template string, args ...any)
	ErrorfContext(ctx context.Context, template string, args ...any)
	ErrorwContext(ctx context.Context, template string, args ...any)
	PanicfContext(ctx context.Context, template string, args ...any)
	PanicwContext(ctx context.Context, template string, args ...any)
}

func NewOtelLogger(zapLogger *zap.Logger) otelLogger {
	sugared := otelzap.New(zapLogger,
		otelzap.WithLoggerProvider(newLoggerProvider(zapLogger.Name(), OtelSanitize())),
		otelzap.WithMinLevel(zapLogger.Level()),
		otelzap.WithCallerDepth(1),
	).Sugar()
	return ctxFieldLogger{sugared}
}

// ctxFieldLogger decorates an *otelzap.SugaredLogger so that every context-aware log
// call also picks up the registered ContextLogFields (see config.go) from ctx and adds
// them to the zap log line.
type ctxFieldLogger struct {
	*otelzap.SugaredLogger
}

// levelEnabled reports whether lvl would actually be written by the underlying zap core.
// Only called for DebugLevel..ErrorLevel: Panic/PanicW below always extract unconditionally
// since, per zap.SugaredLogger.log, Panic/Fatal levels always proceed regardless of the
// core's enabled level (they may panic/exit). This lets disabled-level calls (the
// overwhelming majority in production) skip context-field extraction entirely, at the
// cost of a single cheap comparison (SugaredLogger.Level() is allocation-free).
func (l ctxFieldLogger) levelEnabled(lvl zapcore.Level) bool {
	return lvl >= l.Level()
}

// withContextFields attaches the registered ContextLogFields present in ctx via With,
// for the *f*Context (printf-style) methods which have no keysAndValues slice to append
// to directly. Only called once levelEnabled has confirmed the entry will be written.
func (l ctxFieldLogger) withContextFields(ctx context.Context) *otelzap.SugaredLogger {
	if fields := contextLogFieldArgs(ctx); len(fields) > 0 {
		return l.With(fields...)
	}
	return l.SugaredLogger
}

// appendContextLogFields appends the registered ContextLogFields present in ctx to
// keysAndValues, for the *w*Context (structured) methods. zap.SugaredLogger.sweetenFields
// natively supports mixing strongly-typed Field values into a keysAndValues slice, so this
// avoids the extra SugaredLogger clone that With(...) would otherwise allocate. The combined
// slice is only allocated once a registered key is actually found in ctx (the common case
// where ctx carries none, or only some, of the registered keys is otherwise allocation-free);
// when allocated, a fresh backing array is used so the caller's keysAndValues slice (if
// passed via `...` spread of an existing slice) is never mutated.
func appendContextLogFields(ctx context.Context, keysAndValues []any) []any {
	if ctx == nil {
		return keysAndValues
	}
	specs := ContextLogFields()
	if len(specs) == 0 {
		return keysAndValues
	}
	var combined []any
	for _, s := range specs {
		v := ctx.Value(s.Key)
		if v == nil {
			continue
		}
		if combined == nil {
			combined = make([]any, len(keysAndValues), len(keysAndValues)+len(specs))
			copy(combined, keysAndValues)
		}
		combined = append(combined, zap.Any(s.Name, v))
	}
	if combined == nil {
		return keysAndValues
	}
	return combined
}

// contextLogFieldArgs extracts the registered ContextLogFields present in ctx, returned
// as a flat []any of zap.Field values suitable for SugaredLogger.With or a keysAndValues
// slice. Returns nil, allocation-free, if ctx carries none of the registered keys.
func contextLogFieldArgs(ctx context.Context) []any {
	if ctx == nil {
		return nil
	}
	specs := ContextLogFields()
	if len(specs) == 0 {
		return nil
	}
	var args []any
	for _, s := range specs {
		v := ctx.Value(s.Key)
		if v == nil {
			continue
		}
		if args == nil {
			args = make([]any, 0, len(specs))
		}
		args = append(args, zap.Any(s.Name, v))
	}
	return args
}

func (l ctxFieldLogger) DebugfContext(ctx context.Context, template string, args ...any) {
	if !l.levelEnabled(zapcore.DebugLevel) {
		l.SugaredLogger.DebugfContext(ctx, template, args...)
		return
	}
	l.withContextFields(ctx).DebugfContext(ctx, template, args...)
}

func (l ctxFieldLogger) DebugwContext(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.DebugLevel) {
		l.SugaredLogger.DebugwContext(ctx, msg, keysAndValues...)
		return
	}
	l.SugaredLogger.DebugwContext(ctx, msg, appendContextLogFields(ctx, keysAndValues)...)
}

func (l ctxFieldLogger) InfofContext(ctx context.Context, template string, args ...any) {
	if !l.levelEnabled(zapcore.InfoLevel) {
		l.SugaredLogger.InfofContext(ctx, template, args...)
		return
	}
	l.withContextFields(ctx).InfofContext(ctx, template, args...)
}

func (l ctxFieldLogger) InfowContext(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.InfoLevel) {
		l.SugaredLogger.InfowContext(ctx, msg, keysAndValues...)
		return
	}
	l.SugaredLogger.InfowContext(ctx, msg, appendContextLogFields(ctx, keysAndValues)...)
}

func (l ctxFieldLogger) WarnfContext(ctx context.Context, template string, args ...any) {
	if !l.levelEnabled(zapcore.WarnLevel) {
		l.SugaredLogger.WarnfContext(ctx, template, args...)
		return
	}
	l.withContextFields(ctx).WarnfContext(ctx, template, args...)
}

func (l ctxFieldLogger) WarnwContext(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.WarnLevel) {
		l.SugaredLogger.WarnwContext(ctx, msg, keysAndValues...)
		return
	}
	l.SugaredLogger.WarnwContext(ctx, msg, appendContextLogFields(ctx, keysAndValues)...)
}

func (l ctxFieldLogger) ErrorfContext(ctx context.Context, template string, args ...any) {
	if !l.levelEnabled(zapcore.ErrorLevel) {
		l.SugaredLogger.ErrorfContext(ctx, template, args...)
		return
	}
	l.withContextFields(ctx).ErrorfContext(ctx, template, args...)
}

func (l ctxFieldLogger) ErrorwContext(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.ErrorLevel) {
		l.SugaredLogger.ErrorwContext(ctx, msg, keysAndValues...)
		return
	}
	l.SugaredLogger.ErrorwContext(ctx, msg, appendContextLogFields(ctx, keysAndValues)...)
}

func (l ctxFieldLogger) PanicfContext(ctx context.Context, template string, args ...any) {
	l.withContextFields(ctx).PanicfContext(ctx, template, args...)
}

func (l ctxFieldLogger) PanicwContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.SugaredLogger.PanicwContext(ctx, msg, appendContextLogFields(ctx, keysAndValues)...)
}

func newLoggerProvider(name string, sanitize bool) *spanLoggerProvider {
	return &spanLoggerProvider{
		LoggerProvider: noop.NewLoggerProvider(),
		loggerName:     name,
		sanitize:       sanitize,
	}
}

type spanLoggerProvider struct {
	log.LoggerProvider

	loggerName string
	sanitize   bool
}

func (p *spanLoggerProvider) Logger(name string, options ...log.LoggerOption) log.Logger {
	if p.sanitize {
		return &sanitizedSpanLogger{
			Logger:     p.LoggerProvider.Logger(name, options...),
			loggerName: p.loggerName,
		}
	}

	return &spanLogger{
		Logger:     p.LoggerProvider.Logger(name, options...),
		loggerName: p.loggerName,
	}
}

type spanLogger struct {
	log.Logger

	loggerName string
}

func (l *spanLogger) Emit(ctx context.Context, record log.Record) {
	trace.SpanFromContext(ctx).AddEvent(record.Body().AsString(), trace.WithAttributes(attribute.String(loggerNameKey, l.loggerName)))
}

func (l *spanLogger) Enabled(context.Context, log.EnabledParameters) bool { return true }

type sanitizedSpanLogger struct {
	log.Logger

	loggerName string
}

func (l *sanitizedSpanLogger) Emit(ctx context.Context, record log.Record) {
	// ensure it is printable
	str := FilterPrintableWithMarker(record.Body().AsString())
	trace.SpanFromContext(ctx).AddEvent(str, trace.WithAttributes(attribute.String(loggerNameKey, l.loggerName)))
}

func (l *sanitizedSpanLogger) Enabled(context.Context, log.EnabledParameters) bool { return true }
