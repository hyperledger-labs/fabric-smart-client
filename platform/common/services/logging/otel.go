/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"

	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const loggerNameKey = "logger.name"

type otelLogger interface {
	DebugfContext(ctx context.Context, template string, args ...interface{})
	DebugwContext(ctx context.Context, template string, args ...interface{})
	InfofContext(ctx context.Context, template string, args ...interface{})
	InfowContext(ctx context.Context, template string, args ...interface{})
	WarnfContext(ctx context.Context, template string, args ...interface{})
	WarnwContext(ctx context.Context, template string, args ...interface{})
	ErrorfContext(ctx context.Context, template string, args ...interface{})
	ErrorwContext(ctx context.Context, template string, args ...interface{})
	PanicfContext(ctx context.Context, template string, args ...interface{})
	PanicwContext(ctx context.Context, template string, args ...interface{})
}

func NewOtelLogger(zapLogger *zap.Logger) otelLogger {
	return otelzap.New(zapLogger,
		otelzap.WithLoggerProvider(newLoggerProvider(zapLogger.Name())),
		//otelzap.WithMinLevel(zapLogger.Level()),
		otelzap.WithMinLevel(zapcore.DebugLevel),
	).Sugar()
}

func newLoggerProvider(name string) *spanLoggerProvider {
	return &spanLoggerProvider{
		LoggerProvider: noop.NewLoggerProvider(),
		loggerName:     name,
	}
}

type spanLoggerProvider struct {
	log.LoggerProvider

	loggerName string
}

func (p *spanLoggerProvider) Logger(name string, options ...log.LoggerOption) log.Logger {
	return &spanLogger{
		Logger:     p.LoggerProvider.Logger(name, options...),
		loggerName: p.loggerName,
	}
}

type spanLogger struct {
	log.Logger

	loggerName string
}

func (l *spanLogger) Emit(ctx context.Context, record log.Record) {
	trace.SpanFromContext(ctx).AddEvent(record.Body().AsString(), trace.WithAttributes(attribute.String(loggerNameKey, l.loggerName)))
}

func (l *spanLogger) Enabled(context.Context, log.Record) bool { return true }
