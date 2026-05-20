/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func TestLabels_NewLabels(t *testing.T) {
	t.Parallel()

	keys := []string{"key1", "key2", "key3"}
	labels := tracing.NewLabels(keys)

	require.NotNil(t, labels)
}

func TestLabels_AppendAndToLabels(t *testing.T) {
	t.Parallel()

	keys := []string{"status", "method"}
	labels := tracing.NewLabels(keys)

	labels.Append(
		tracing.String("status", "success"),
		tracing.String("method", "POST"),
	)

	result := labels.ToLabels()
	require.Len(t, result, 4)
}

func TestTracer_Start_CreatesSpanContext(t *testing.T) {
	t.Parallel()

	tracer := newTestTracer(t, "status")
	ctx := t.Context()
	_, span := tracer.Start(ctx, "operation")

	require.NotNil(t, span)

	span.End()
}

func TestTracer_Start_WithAttributes(t *testing.T) {
	t.Parallel()

	tracer := newTestTracer(t, "status")
	ctx := t.Context()
	_, span := tracer.Start(
		ctx,
		"operation",
		tracing.WithAttributes(tracing.String("status", "running")),
	)

	require.NotNil(t, span)

	span.End()
}

func TestSpan_SetAttributes(t *testing.T) {
	t.Parallel()

	tracer := newTestTracer(t, "status")
	ctx := t.Context()
	_, span := tracer.Start(ctx, "operation")

	span.SetAttributes(
		tracing.String("status", "complete"),
		tracing.Int("code", 200),
		tracing.Bool("success", true),
	)

	span.End()
	require.NotNil(t, span)
}

func TestSpan_AddEvent(t *testing.T) {
	t.Parallel()

	tracer := newTestTracer(t, "level")
	ctx := t.Context()
	_, span := tracer.Start(ctx, "operation")

	span.AddEvent("event1", tracing.WithAttributes(tracing.String("level", "info")))
	span.AddEvent("event2")

	span.End()
	require.NotNil(t, span)
}

func TestSpan_End_WithTimestamp(t *testing.T) {
	t.Parallel()

	tracer := newTestTracer(t, "status")
	ctx := t.Context()
	_, span := tracer.Start(ctx, "operation")

	span.End()
	require.NotNil(t, span)
}

func TestSpan_CompleteLifecycle(t *testing.T) {
	t.Parallel()

	tracer := newTestTracer(t, "status", "error")
	ctx := t.Context()
	newCtx, span := tracer.Start(
		ctx,
		"complex-operation",
		tracing.WithAttributes(tracing.String("status", "started")),
	)

	span.SetAttributes(
		tracing.String("status", "processing"),
		tracing.Int("steps", 3),
	)

	span.AddEvent("step_1_completed", tracing.WithAttributes(tracing.String("error", "none")))
	span.AddEvent("step_2_completed")

	span.End()

	require.NotNil(t, newCtx)
}
