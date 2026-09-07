/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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

// NewOtelLogger returns a logger whose *Context methods log through zapLogger and, when ctx
// carries a recording span, additionally record the message as an event on that span (and set
// the span status to Error for Error level and above).
//
// Span events are the only OTel signal produced here: the traces are the destination, so no
// log-signal LoggerProvider is involved.
func NewOtelLogger(zapLogger *zap.Logger) otelLogger {
	return ctxFieldLogger{
		SugaredLogger: zapLogger.Sugar(),
		loggerName:    zapLogger.Name(),
		sanitize:      OtelSanitize(),
	}
}

// ctxFieldLogger decorates a *zap.SugaredLogger so that every context-aware log call also
// picks up the registered ContextLogFields (see config.go) from ctx and adds them to the zap
// log line, and mirrors the message onto the span in ctx.
type ctxFieldLogger struct {
	*zap.SugaredLogger

	loggerName string
	sanitize   bool
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

// spanEvent records msg as an event on the span in ctx, if that span is recording. Nothing is
// formatted or sanitized when there is no recording span, which is the common case.
func (l ctxFieldLogger) spanEvent(ctx context.Context, lvl zapcore.Level, msg string) {
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		l.addEvent(span, lvl, msg)
	}
}

// spanEventf is spanEvent for the printf-style methods: it only pays for fmt.Sprintf when
// there is a recording span to receive the result.
func (l ctxFieldLogger) spanEventf(ctx context.Context, lvl zapcore.Level, template string, args []any) {
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		l.addEvent(span, lvl, fmt.Sprintf(template, args...))
	}
}

func (l ctxFieldLogger) addEvent(span trace.Span, lvl zapcore.Level, msg string) {
	if lvl >= zapcore.ErrorLevel {
		span.SetStatus(codes.Error, msg)
	}
	if l.sanitize {
		// ensure it is printable
		msg = FilterPrintableWithMarker(msg)
	}
	span.AddEvent(msg, trace.WithAttributes(attribute.String(loggerNameKey, l.loggerName)))
}

// withContextFields attaches the registered ContextLogFields present in ctx via With,
// for the *f*Context (printf-style) methods which have no keysAndValues slice to append
// to directly. Only called once levelEnabled has confirmed the entry will be written.
func (l ctxFieldLogger) withContextFields(ctx context.Context) *zap.SugaredLogger {
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
		l.Debugf(template, args...)
		return
	}
	l.spanEventf(ctx, zapcore.DebugLevel, template, args)
	l.withContextFields(ctx).Debugf(template, args...)
}

func (l ctxFieldLogger) DebugwContext(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.DebugLevel) {
		l.Debugw(msg, keysAndValues...)
		return
	}
	l.spanEvent(ctx, zapcore.DebugLevel, msg)
	l.Debugw(msg, appendContextLogFields(ctx, keysAndValues)...)
}

func (l ctxFieldLogger) InfofContext(ctx context.Context, template string, args ...any) {
	if !l.levelEnabled(zapcore.InfoLevel) {
		l.Infof(template, args...)
		return
	}
	l.spanEventf(ctx, zapcore.InfoLevel, template, args)
	l.withContextFields(ctx).Infof(template, args...)
}

func (l ctxFieldLogger) InfowContext(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.InfoLevel) {
		l.Infow(msg, keysAndValues...)
		return
	}
	l.spanEvent(ctx, zapcore.InfoLevel, msg)
	l.Infow(msg, appendContextLogFields(ctx, keysAndValues)...)
}

func (l ctxFieldLogger) WarnfContext(ctx context.Context, template string, args ...any) {
	if !l.levelEnabled(zapcore.WarnLevel) {
		l.Warnf(template, args...)
		return
	}
	l.spanEventf(ctx, zapcore.WarnLevel, template, args)
	l.withContextFields(ctx).Warnf(template, args...)
}

func (l ctxFieldLogger) WarnwContext(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.WarnLevel) {
		l.Warnw(msg, keysAndValues...)
		return
	}
	l.spanEvent(ctx, zapcore.WarnLevel, msg)
	l.Warnw(msg, appendContextLogFields(ctx, keysAndValues)...)
}

func (l ctxFieldLogger) ErrorfContext(ctx context.Context, template string, args ...any) {
	if !l.levelEnabled(zapcore.ErrorLevel) {
		l.Errorf(template, args...)
		return
	}
	l.spanEventf(ctx, zapcore.ErrorLevel, template, args)
	l.withContextFields(ctx).Errorf(template, args...)
}

func (l ctxFieldLogger) ErrorwContext(ctx context.Context, msg string, keysAndValues ...any) {
	if !l.levelEnabled(zapcore.ErrorLevel) {
		l.Errorw(msg, keysAndValues...)
		return
	}
	l.spanEvent(ctx, zapcore.ErrorLevel, msg)
	l.Errorw(msg, appendContextLogFields(ctx, keysAndValues)...)
}

func (l ctxFieldLogger) PanicfContext(ctx context.Context, template string, args ...any) {
	l.spanEventf(ctx, zapcore.PanicLevel, template, args)
	l.withContextFields(ctx).Panicf(template, args...)
}

func (l ctxFieldLogger) PanicwContext(ctx context.Context, msg string, keysAndValues ...any) {
	l.spanEvent(ctx, zapcore.PanicLevel, msg)
	l.Panicw(msg, appendContextLogFields(ctx, keysAndValues)...)
}
