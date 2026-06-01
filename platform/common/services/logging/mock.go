/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Ensure MockLogger satisfies the Logger interface
var _ Logger = (*MockLogger)(nil)

type MockLogger struct{}

func (m *MockLogger) Named(name string) Logger {
	fmt.Println("Named:", name)
	return m
}

func (m *MockLogger) Debug(args ...any) {
	fmt.Println("DEBUG:", fmt.Sprint(args...))
}

func (m *MockLogger) Debugf(format string, args ...any) {
	fmt.Printf("DEBUG: "+format+"\n", args...)
}

func (m *MockLogger) Error(args ...any) {
	fmt.Println("ERROR:", fmt.Sprint(args...))
}

func (m *MockLogger) Errorf(format string, args ...any) {
	fmt.Printf("ERROR: "+format+"\n", args...)
}

func (m *MockLogger) Fatal(args ...any) {
	fmt.Println("FATAL:", fmt.Sprint(args...))
}

func (m *MockLogger) Fatalf(format string, args ...any) {
	fmt.Printf("FATAL: "+format+"\n", args...)
}

func (m *MockLogger) Info(args ...any) {
	fmt.Println("INFO:", fmt.Sprint(args...))
}

func (m *MockLogger) Infof(format string, args ...any) {
	fmt.Printf("INFO: "+format+"\n", args...)
}

func (m *MockLogger) Panic(args ...any) {
	fmt.Println("PANIC:", fmt.Sprint(args...))
}

func (m *MockLogger) Panicf(format string, args ...any) {
	fmt.Printf("PANIC: "+format+"\n", args...)
}

func (m *MockLogger) Warn(args ...any) {
	fmt.Println("WARN:", fmt.Sprint(args...))
}

func (m *MockLogger) Warnf(format string, args ...any) {
	fmt.Printf("WARN: "+format+"\n", args...)
}

func (m *MockLogger) IsEnabledFor(level zapcore.Level) bool {
	// Implement logic to check if the given log level is enabled
	return true
}

func (m *MockLogger) Warnw(format string, args ...any) {
	fmt.Printf("WARN: "+format+"\n", args...)
}

func (m *MockLogger) Warningf(format string, args ...any) {
	fmt.Printf("WARNING: "+format+"\n", args...)
}

func (m *MockLogger) Errorw(format string, args ...any) {
	fmt.Printf("ERROR: "+format+"\n", args...)
}

func (m *MockLogger) With(args ...any) Logger {
	fmt.Println("With:", fmt.Sprint(args...))
	return m
}

func (m *MockLogger) DebugfContext(_ context.Context, template string, args ...any) {
	m.Debugf(template, args...)
}

func (m *MockLogger) DebugwContext(_ context.Context, template string, args ...any) {
	m.Debugf(template, args...)
}

func (m *MockLogger) InfofContext(_ context.Context, template string, args ...any) {
	m.Infof(template, args...)
}

func (m *MockLogger) InfowContext(_ context.Context, template string, args ...any) {
	m.Infof(template, args...)
}

func (m *MockLogger) WarnwContext(_ context.Context, template string, args ...any) {
	m.Warnf(template, args...)
}

func (m *MockLogger) WarnfContext(_ context.Context, template string, args ...any) {
	m.Warnf(template, args...)
}

func (m *MockLogger) ErrorfContext(_ context.Context, template string, args ...any) {
	m.Errorf(template, args...)
}

func (m *MockLogger) ErrorwContext(_ context.Context, template string, args ...any) {
	m.Errorf(template, args...)
}

func (m *MockLogger) PanicfContext(_ context.Context, template string, args ...any) {
	m.Panicf(template, args...)
}

func (m *MockLogger) PanicwContext(_ context.Context, template string, args ...any) {
	m.Panicf(template, args...)
}

func (m *MockLogger) Zap() *zap.Logger {
	return nil
}
