/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// recordingSpanCtx returns a context carrying a live, recording SDK span plus the recorder
// holding it. Call end() before inspecting: a span's events are only exported once it ends.
func recordingSpanCtx(t *testing.T) (ctx context.Context, ended func() tracetest.SpanStub) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, span := tp.Tracer("test").Start(context.Background(), "root")
	return ctx, func() tracetest.SpanStub {
		span.End()
		stubs := rec.Ended()
		require.Len(t, stubs, 1)
		return tracetest.SpanStubFromReadOnlySpan(stubs[0])
	}
}

func otelTestLogger(t *testing.T, lvl zapcore.Level, sanitize bool) (Logger, *observer.ObservedLogs) {
	t.Helper()
	t.Cleanup(func() { Init(Config{}) })
	Init(Config{OtelSanitize: sanitize})
	core, observed := observer.New(lvl)
	return newLogger(zap.New(core).Named("my.logger")), observed
}

func TestSpanEvent_RecordedWithLoggerName(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	logger, observed := otelTestLogger(t, zapcore.InfoLevel, false)
	ctx, ended := recordingSpanCtx(t)

	logger.InfowContext(ctx, "hello")

	span := ended()
	require.Len(t, span.Events, 1)
	require.Equal(t, "hello", span.Events[0].Name)
	require.Len(t, span.Events[0].Attributes, 1)
	require.Equal(t, loggerNameKey, string(span.Events[0].Attributes[0].Key))
	require.Equal(t, "my.logger", span.Events[0].Attributes[0].Value.AsString())
	// the zap line is still written, independently of the span
	require.Len(t, observed.All(), 1)
}

func TestSpanEvent_PrintfMessageIsFormatted(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	logger, _ := otelTestLogger(t, zapcore.InfoLevel, false)
	ctx, ended := recordingSpanCtx(t)

	logger.InfofContext(ctx, "tx %s at %d", "abc", 7)

	span := ended()
	require.Len(t, span.Events, 1)
	require.Equal(t, "tx abc at 7", span.Events[0].Name)
}

func TestSpanEvent_ErrorSetsSpanStatus(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	logger, _ := otelTestLogger(t, zapcore.InfoLevel, false)
	ctx, ended := recordingSpanCtx(t)

	logger.InfowContext(ctx, "fine")
	logger.ErrorwContext(ctx, "boom")

	span := ended()
	require.Len(t, span.Events, 2)
	require.Equal(t, codes.Error, span.Status.Code)
	require.Equal(t, "boom", span.Status.Description)
}

func TestSpanEvent_NotRecordedBelowLevel(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	logger, observed := otelTestLogger(t, zapcore.InfoLevel, false)
	ctx, ended := recordingSpanCtx(t)

	logger.DebugwContext(ctx, "suppressed")
	logger.DebugfContext(ctx, "suppressed %d", 1)

	require.Empty(t, ended().Events)
	require.Empty(t, observed.All())
}

func TestSpanEvent_SanitizedWhenConfigured(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	logger, _ := otelTestLogger(t, zapcore.InfoLevel, true)
	ctx, ended := recordingSpanCtx(t)

	logger.InfowContext(ctx, "ok\x00bad")

	span := ended()
	require.Len(t, span.Events, 1)
	require.Equal(t, FilterPrintableWithMarker("ok\x00bad"), span.Events[0].Name)
	require.NotContains(t, span.Events[0].Name, "\x00")
}

func TestSpanEvent_NoSpanInContextIsSafe(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	logger, observed := otelTestLogger(t, zapcore.InfoLevel, false)

	logger.InfowContext(context.Background(), "no span")

	require.Len(t, observed.All(), 1)
}
