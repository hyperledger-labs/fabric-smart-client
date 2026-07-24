/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// ctxKey is a distinct comparable type used as a context key in these tests, following
// the pattern used elsewhere in the repo (e.g. grpc/logging, web/server/middleware).
type ctxKey struct{ name string }

func newObservedLogger() (Logger, *observer.ObservedLogs) {
	core, observed := observer.New(zapcore.DebugLevel)
	zl := zap.New(core, zap.AddCaller())
	return newLogger(zl), observed
}

func findField(fields []zapcore.Field, name string) (zapcore.Field, bool) {
	for _, f := range fields {
		if f.Key == name {
			return f, true
		}
	}
	return zapcore.Field{}, false
}

func TestContextLogFields_ExtractedIntoLogLine(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	key := ctxKey{"present"}
	Init(Config{ContextLogFields: []ContextLogField{{Key: key, Name: "test.present"}}})

	logger, observed := newObservedLogger()
	ctx := context.WithValue(context.Background(), key, "some-value")

	logger.InfowContext(ctx, "hello")

	entries := observed.All()
	require.Len(t, entries, 1)
	field, ok := findField(entries[0].Context, "test.present")
	require.True(t, ok, "expected field %q in %+v", "test.present", entries[0].Context)
	require.Equal(t, "some-value", field.String)
}

func TestContextLogFields_AbsentKeyNoField(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	key := ctxKey{"absent"}
	Init(Config{ContextLogFields: []ContextLogField{{Key: key, Name: "test.absent"}}})

	logger, observed := newObservedLogger()

	logger.InfowContext(context.Background(), "hello")

	entries := observed.All()
	require.Len(t, entries, 1)
	_, ok := findField(entries[0].Context, "test.absent")
	require.False(t, ok, "did not expect field %q in %+v", "test.absent", entries[0].Context)
}

func TestContextLogFieldArgs_NilContextNoPanic(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	key := ctxKey{"nilctx"}
	Init(Config{ContextLogFields: []ContextLogField{{Key: key, Name: "test.nilctx"}}})

	var nilCtx context.Context
	require.NotPanics(t, func() {
		args := contextLogFieldArgs(nilCtx)
		require.Nil(t, args)
	})
}

func TestContextLogFields_OnlyPresentKeysAdded(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	presentKey := ctxKey{"multi.present"}
	absentKey := ctxKey{"multi.absent"}
	Init(Config{ContextLogFields: []ContextLogField{
		{Key: presentKey, Name: "test.multi.present"},
		{Key: absentKey, Name: "test.multi.absent"},
	}})

	logger, observed := newObservedLogger()
	ctx := context.WithValue(context.Background(), presentKey, "value")

	logger.InfowContext(ctx, "hello")

	entries := observed.All()
	require.Len(t, entries, 1)

	_, ok := findField(entries[0].Context, "test.multi.present")
	require.True(t, ok)
	_, ok = findField(entries[0].Context, "test.multi.absent")
	require.False(t, ok)
}

func TestContextLogFields_SurviveWithAndNamed(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	key := ctxKey{"survive"}
	// Register the field only after the loggers below have already been created and
	// derived, proving extraction is dynamic (read at call time, not construction time).
	core, observed := observer.New(zapcore.DebugLevel)
	zl := zap.New(core, zap.AddCaller())
	base := newLogger(zl)
	derived := base.Named("child").With("k", "v")

	Init(Config{ContextLogFields: []ContextLogField{{Key: key, Name: "test.survive"}}})

	ctx := context.WithValue(context.Background(), key, "still-works")
	derived.InfowContext(ctx, "hello")

	entries := observed.All()
	require.Len(t, entries, 1)
	field, ok := findField(entries[0].Context, "test.survive")
	require.True(t, ok)
	require.Equal(t, "still-works", field.String)
}

func TestRegisterContextLogField_DuplicatePanics(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	key1 := ctxKey{"dup1"}
	key2 := ctxKey{"dup2"}
	RegisterContextLogField("test.dup.unique", key1)

	require.Panics(t, func() {
		RegisterContextLogField("test.dup.unique", key2)
	})
}

func TestInit_ContextLogFields_Idempotent(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	firstKey := ctxKey{"idempotent.first"}
	secondKey := ctxKey{"idempotent.second"}

	require.NotPanics(t, func() {
		Init(Config{ContextLogFields: []ContextLogField{{Key: firstKey, Name: "test.idempotent"}}})
		Init(Config{ContextLogFields: []ContextLogField{{Key: secondKey, Name: "test.idempotent"}}})
	})

	logger, observed := newObservedLogger()
	ctx := context.WithValue(context.Background(), secondKey, "second-value")
	logger.InfowContext(ctx, "hello")

	entries := observed.All()
	require.Len(t, entries, 1)
	field, ok := findField(entries[0].Context, "test.idempotent")
	require.True(t, ok)
	require.Equal(t, "second-value", field.String)
}

func TestContextLogFields_CallerAccuracy(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	key := ctxKey{"caller"}
	Init(Config{ContextLogFields: []ContextLogField{{Key: key, Name: "test.caller"}}})

	logger, observed := newObservedLogger()
	ctx := context.WithValue(context.Background(), key, "value")

	logger.InfowContext(ctx, "hello") // the call site under test

	entries := observed.All()
	require.Len(t, entries, 1)
	require.True(t, entries[0].Caller.Defined)
	require.True(t, strings.HasSuffix(entries[0].Caller.File, "context_fields_test.go"),
		"expected caller in context_fields_test.go, got %s", entries[0].Caller.File)
}
