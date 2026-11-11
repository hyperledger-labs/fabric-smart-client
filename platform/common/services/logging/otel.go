/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	loggerNameKey = "logger.name"
)

// spanEvent records msg as an event on the span in ctx, if that span is recording, and
// sets the span status to Error at ErrorLevel. Span events are the only OpenTelemetry
// signal produced here: no log-signal LoggerProvider is involved.
func (l *logger) spanEvent(ctx context.Context, level zapcore.Level, msg string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	if level >= zapcore.ErrorLevel {
		span.SetStatus(codes.Error, msg)
	}
	// Read at the call. Package-level loggers are created during package
	// initialization, and Init applies logging.otel.sanitize later.
	if otelSanitize() {
		msg = filterPrintableWithMarker(msg)
	}
	span.AddEvent(msg, trace.WithAttributes(attribute.String(loggerNameKey, l.name)))
}

// appendContextLogFields appends the registered context log fields present in ctx to
// keysAndValues as zap fields, which zap accepts mixed with key/value pairs. It allocates
// only when ctx carries a registered key, and then into a new slice, so the caller's
// slice is never changed.
func appendContextLogFields(ctx context.Context, keysAndValues []any) []any {
	specs := contextLogFields()
	if ctx == nil || len(specs) == 0 {
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
