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
