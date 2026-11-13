/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	loggerNameKey = "logger.name"
)

// spanEvent records msg as an event on the span in ctx, if that span is recording, and
// sets the span status to Error at ErrorLevel. Span events are the only OpenTelemetry
// signal produced here: no log-signal LoggerProvider is involved.
func (l *logger) spanEvent(ctx context.Context, level Level, msg string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	if level >= ErrorLevel {
		span.SetStatus(codes.Error, msg)
	}
	// Read at the call. Package-level loggers are created during package
	// initialization, and Init applies logging.otel.sanitize later.
	if otelSanitize() {
		msg = filterPrintableWithMarker(msg)
	}
	span.AddEvent(msg, trace.WithAttributes(attribute.String(loggerNameKey, l.name)))
}

// addContextLogFields adds to r the registered context log fields present in ctx.
func addContextLogFields(ctx context.Context, r *slog.Record) {
	for _, f := range contextLogFields() {
		if v := ctx.Value(f.Key); v != nil {
			r.AddAttrs(slog.Any(f.Name, v))
		}
	}
}
