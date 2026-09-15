/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// levelObservedLogger returns a logger whose core only writes at lvl and above, so the
// disabled-level branch of the *Context methods can be exercised.
func levelObservedLogger(lvl zapcore.Level) (Logger, *observer.ObservedLogs) {
	core, observed := observer.New(lvl)

	return newLogger(zap.New(core)), observed
}

// contextMethod names one of the eight level-guarded *Context methods, along with the
// level it logs at and whether it takes a printf template or a keysAndValues slice.
type contextMethod struct {
	name       string
	level      zapcore.Level
	structured bool
	call       func(l Logger, ctx context.Context)
}

func contextMethods() []contextMethod {
	return []contextMethod{
		{"DebugfContext", zapcore.DebugLevel, false, func(l Logger, ctx context.Context) {
			l.DebugfContext(ctx, "hello %s", "world")
		}},
		{"DebugwContext", zapcore.DebugLevel, true, func(l Logger, ctx context.Context) {
			l.DebugwContext(ctx, "hello")
		}},
		{"InfofContext", zapcore.InfoLevel, false, func(l Logger, ctx context.Context) {
			l.InfofContext(ctx, "hello %s", "world")
		}},
		{"InfowContext", zapcore.InfoLevel, true, func(l Logger, ctx context.Context) {
			l.InfowContext(ctx, "hello")
		}},
		{"WarnfContext", zapcore.WarnLevel, false, func(l Logger, ctx context.Context) {
			l.WarnfContext(ctx, "hello %s", "world")
		}},
		{"WarnwContext", zapcore.WarnLevel, true, func(l Logger, ctx context.Context) {
			l.WarnwContext(ctx, "hello")
		}},
		{"ErrorfContext", zapcore.ErrorLevel, false, func(l Logger, ctx context.Context) {
			l.ErrorfContext(ctx, "hello %s", "world")
		}},
		{"ErrorwContext", zapcore.ErrorLevel, true, func(l Logger, ctx context.Context) {
			l.ErrorwContext(ctx, "hello")
		}},
	}
}

// TestContextMethods_EnabledLevelAttachesFields checks that, when the level is enabled,
// each method writes its line and picks up the registered context fields.
func TestContextMethods_EnabledLevelAttachesFields(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	for _, m := range contextMethods() { //nolint:paralleltest // mutates the shared global context-field registry
		t.Run(m.name, func(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
			t.Cleanup(resetContextLogFields)
			key := ctxKey{"enabled"}
			Init(Config{ContextLogFields: []ContextLogField{{Key: key, Name: "test.enabled"}}})

			logger, observed := levelObservedLogger(zapcore.DebugLevel)
			m.call(logger, context.WithValue(context.Background(), key, "some-value"))

			entries := observed.All()
			require.Len(t, entries, 1)
			assert.Equal(t, m.level, entries[0].Level)

			field, ok := findField(entries[0].Context, "test.enabled")
			require.True(t, ok, "expected field in %+v", entries[0].Context)
			assert.Equal(t, "some-value", field.String)
		})
	}
}

// TestContextMethods_DisabledLevelSkipsExtraction checks the other side of the branch: below
// the core's level nothing is written at all, so no context fields are extracted.
func TestContextMethods_DisabledLevelSkipsExtraction(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	for _, m := range contextMethods() { //nolint:paralleltest // mutates the shared global context-field registry
		t.Run(m.name, func(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
			t.Cleanup(resetContextLogFields)
			key := ctxKey{"disabled"}
			Init(Config{ContextLogFields: []ContextLogField{{Key: key, Name: "test.disabled"}}})

			logger, observed := levelObservedLogger(m.level + 1)
			m.call(logger, context.WithValue(context.Background(), key, "some-value"))

			assert.Empty(t, observed.All(), "nothing is written below the core's level")
		})
	}
}

// TestContextMethods_NoRegisteredFields checks the early return taken when nothing is
// registered: the line is still written, just without any context fields.
func TestContextMethods_NoRegisteredFields(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	for _, m := range contextMethods() { //nolint:paralleltest // mutates the shared global context-field registry
		t.Run(m.name, func(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
			t.Cleanup(resetContextLogFields)
			Init(Config{})

			logger, observed := levelObservedLogger(zapcore.DebugLevel)
			m.call(logger, context.Background())

			entries := observed.All()
			require.Len(t, entries, 1)
			assert.Empty(t, entries[0].Context, "no fields are attached when none are registered")
		})
	}
}

// TestAppendContextLogFields_LeavesCallerSliceUnchanged checks the documented promise that
// the caller's keysAndValues slice is never mutated when context fields are appended.
func TestAppendContextLogFields_LeavesCallerSliceUnchanged(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	t.Cleanup(resetContextLogFields)
	key := ctxKey{"append"}
	Init(Config{ContextLogFields: []ContextLogField{{Key: key, Name: "test.append"}}})

	original := []any{"k", "v"}
	ctx := context.WithValue(context.Background(), key, "some-value")

	combined := appendContextLogFields(ctx, original)

	assert.Equal(t, []any{"k", "v"}, original, "the caller's slice is untouched")
	require.Len(t, combined, len(original)+1, "one zap.Field is appended to a copy")
	assert.Equal(t, original, combined[:len(original)], "the caller's entries come first")
}

// TestAppendContextLogFields_NoRegisteredFields checks the early return when the registry
// is empty: the caller's slice is handed straight back.
func TestAppendContextLogFields_NoRegisteredFields(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	t.Cleanup(resetContextLogFields)
	Init(Config{})

	original := []any{"k", "v"}
	assert.Equal(t, original, appendContextLogFields(context.Background(), original))
	assert.Equal(t, original, appendContextLogFields(nil, original)) //nolint:staticcheck // nil context is the case under test
}

// TestContextLogFieldArgs_NoRegisteredFields covers the early return when nothing is
// registered, which the existing nil-context and absent-key tests do not reach.
func TestContextLogFieldArgs_NoRegisteredFields(t *testing.T) { //nolint:paralleltest // mutates the shared global context-field registry
	t.Cleanup(resetContextLogFields)
	Init(Config{})

	assert.Nil(t, contextLogFieldArgs(context.Background()))
}
