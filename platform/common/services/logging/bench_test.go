/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"testing"

	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func observerCore(lvl zapcore.Level) (zapcore.Core, *observer.ObservedLogs) {
	return observer.New(lvl)
}

type benchKey struct{ n int }

var benchRegCounter int

// setupBenchRegistry registers n new context log fields (each call uses a fresh generation
// of keys, so repeated calls across benchmarks in this file keep growing the shared global
// registry rather than colliding) and returns a context carrying values for all of them.
func setupBenchRegistry(n int) context.Context {
	ctx := context.Background()
	gen := benchRegCounter
	benchRegCounter++
	for i := range n {
		k := benchKey{gen*1000 + i}
		RegisterContextLogField(benchFieldName(gen, i), k)
		ctx = context.WithValue(ctx, k, i)
	}
	return ctx
}

func benchFieldName(gen, i int) string {
	return "bench.field." + string(rune('a'+gen)) + string(rune('a'+i))
}

// registerBenchFields registers n new context log fields (a fresh generation of keys, as in
// setupBenchRegistry) without putting any of them into a context, for benchmarks that need a
// populated registry but an otherwise plain context (e.g. context.Background()).
func registerBenchFields(n int) {
	gen := benchRegCounter
	benchRegCounter++
	for i := range n {
		RegisterContextLogField(benchFieldName(gen, i), benchKey{gen*1000 + i})
	}
}

func BenchmarkDebugfContext_Disabled_NoFields(b *testing.B) {
	// NopCore is always disabled; use an Info-level core with debug calls instead for a realistic "disabled but real core" case.
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.DebugfContext(ctx, "msg %d", i)
	}
}

func BenchmarkDebugfContext_Disabled_With3Fields(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	ctx := setupBenchRegistry(3)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.DebugfContext(ctx, "msg %d", i)
	}
}

func BenchmarkInfowContext_Enabled_NoFields(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.InfowContext(ctx, "msg", "i", i)
	}
}

func BenchmarkInfowContext_Enabled_With3Fields(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	ctx := setupBenchRegistry(3)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.InfowContext(ctx, "msg", "i", i)
	}
}

// BenchmarkInfowContext_Enabled_3FieldsRegistered_NoneInContext measures the case where
// fields are registered globally but the request's context carries none of them (e.g. a
// background job, or a request that never populated the optional fields). This is expected
// to be the common case in a real deployment with several optional context fields, so it
// must not pay for the fields it doesn't have.
func BenchmarkInfowContext_Enabled_3FieldsRegistered_NoneInContext(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	registerBenchFields(3)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.InfowContext(ctx, "msg", "i", i)
	}
}

// BenchmarkInfowContext_Enabled_3FieldsRegistered_OneInContext measures the mixed case where
// only some of the registered fields are present in ctx.
func BenchmarkInfowContext_Enabled_3FieldsRegistered_OneInContext(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	gen := benchRegCounter
	benchRegCounter++
	keys := make([]benchKey, 3)
	for i := range keys {
		keys[i] = benchKey{gen*1000 + i}
		RegisterContextLogField(benchFieldName(gen, i), keys[i])
	}
	ctx := context.WithValue(context.Background(), keys[0], 0)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.InfowContext(ctx, "msg", "i", i)
	}
}

func BenchmarkInfowContext_Disabled_With3Fields(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	ctx := setupBenchRegistry(3)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.DebugwContext(ctx, "msg", "i", i)
	}
}

func BenchmarkInfofContext_Enabled_With3Fields(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	ctx := setupBenchRegistry(3)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.InfofContext(ctx, "msg %d", i)
	}
}

// BenchmarkInfowContext_Enabled_With10Fields and With30Fields measure how the cost of
// context-field extraction scales with the size of the global registry, since real
// deployments may register many optional fields (request id, tenant, session, ...) even
// though a given log call's context will typically carry only a handful of them.
func BenchmarkInfowContext_Enabled_With10Fields(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	ctx := setupBenchRegistry(10)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.InfowContext(ctx, "msg", "i", i)
	}
}

func BenchmarkInfowContext_Enabled_With30Fields(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	l := newLogger(zl)
	ctx := setupBenchRegistry(30)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.InfowContext(ctx, "msg", "i", i)
	}
}

// Baseline: raw otelzap.SugaredLogger with no ctxFieldLogger decorator at all, to isolate
// the decorator's own overhead from otelzap's baseline cost.
func BenchmarkInfowContext_RawOtelzap_NoDecorator(b *testing.B) {
	core, _ := observerCore(zapcore.InfoLevel)
	zl := zap.New(core)
	sugared := otelzap.New(zl, otelzap.WithMinLevel(zl.Level())).Sugar()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sugared.InfowContext(ctx, "msg", "i", i)
	}
}
