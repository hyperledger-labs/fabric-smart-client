/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// recordingSpanCtx returns a context carrying a live, recording SDK span plus the recorder
// holding it. Call end() before inspecting: a span's events are only exported once it ends.
func recordingSpanCtx(t *testing.T) (ctx context.Context, ended func() tracetest.SpanStub) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, span := tp.Tracer("test").Start(t.Context(), "root")
	return ctx, func() tracetest.SpanStub {
		span.End()
		stubs := rec.Ended()
		require.Len(t, stubs, 1)
		return tracetest.SpanStubFromReadOnlySpan(stubs[0])
	}
}

func otelTestLogger(t *testing.T, lvl zapcore.Level, sanitize bool) (Logger, *observer.ObservedLogs) {
	t.Helper()
	t.Cleanup(func() { require.NoError(t, Init(Config{Writer: os.Stderr})) })
	// Build the logger before Init. Package-level loggers are created the same way,
	// and sanitize is read when the span event is recorded.
	core, observed := observer.New(lvl)
	logger := newZapLogger(core, "my.logger")
	require.NoError(t, Init(Config{OtelSanitize: sanitize}))
	return logger, observed
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
	require.Equal(t, filterPrintableWithMarker("ok\x00bad"), span.Events[0].Name)
	require.NotContains(t, span.Events[0].Name, "\x00")
}

func TestSpanEvent_UnsanitizedWhenDisabledAfterCreate(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	logger, _ := otelTestLogger(t, zapcore.InfoLevel, false)
	ctx, ended := recordingSpanCtx(t)
	logger.InfowContext(ctx, "a\x00b")

	span := ended()
	require.Len(t, span.Events, 1)
	require.Equal(t, "a\x00b", span.Events[0].Name)
}

func TestSpanEvent_SanitizeFollowsLaterChanges(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	t.Cleanup(func() { require.NoError(t, Init(Config{Writer: os.Stderr})) })
	core, _ := observer.New(zapcore.InfoLevel)
	logger := newZapLogger(core, "")
	ctx, ended := recordingSpanCtx(t)

	require.NoError(t, Init(Config{OtelSanitize: true}))
	logger.InfowContext(ctx, "a\x00b")
	require.NoError(t, Init(Config{OtelSanitize: false}))
	logger.InfowContext(ctx, "a\x00b")

	span := ended()
	require.Len(t, span.Events, 2)
	require.Equal(t, "[nonprintable] ab", span.Events[0].Name)
	require.Equal(t, "a\x00b", span.Events[1].Name)
}

func TestSpanEvent_NoSpanInContextIsSafe(t *testing.T) { //nolint:paralleltest // mutates the shared global config
	logger, observed := otelTestLogger(t, zapcore.InfoLevel, false)

	logger.InfowContext(t.Context(), "no span")

	require.Len(t, observed.All(), 1)
}

// TestTraceIDs checks that a line written with a valid span context in its context carries
// the span's trace and span IDs, whether or not the span is sampled, and that other lines
// carry neither.
func TestTraceIDs(t *testing.T) {
	t.Parallel()

	sc := trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}}
	sampled := sc
	sampled.TraceFlags = trace.FlagsSampled
	for name, tc := range map[string]struct {
		log  func(l Logger)
		want map[string]any
	}{
		"sampled": {
			log: func(l Logger) {
				l.InfofContext(trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(sampled)), "x")
			},
			want: map[string]any{"trace_id": sc.TraceID.String(), "span_id": sc.SpanID.String()},
		},
		"not sampled": {
			log: func(l Logger) {
				l.InfowContext(trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(sc)), "x")
			},
			want: map[string]any{"trace_id": sc.TraceID.String(), "span_id": sc.SpanID.String()},
		},
		"no span context": {log: func(l Logger) { l.InfofContext(t.Context(), "x") }, want: map[string]any{}},
		"no context":      {log: func(l Logger) { l.Infof("x") }, want: map[string]any{}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			core, observed := observer.New(zapcore.InfoLevel)
			tc.log(newZapLogger(core, ""))
			require.Len(t, observed.All(), 1)
			require.Equal(t, tc.want, observed.All()[0].ContextMap())
		})
	}
}
