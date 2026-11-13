/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"fmt"
	"strings"
)

var _ Logger = (*MockLogger)(nil)

// MockLogger is a [Logger] for tests that prints every line to standard output, with its
// level, and reports every level as enabled. Embed it to override single methods; Named
// and With return the MockLogger itself, without the overrides.
type MockLogger struct{}

func (m *MockLogger) Named(name string) Logger {
	fmt.Println("Named:", name)
	return m
}

func (m *MockLogger) With(args ...any) Logger {
	fmt.Println("With:", fmt.Sprint(args...))
	return m
}

func (*MockLogger) IsEnabledFor(Level) bool { return true }

func (*MockLogger) Debug(args ...any)                 { fmt.Println("DEBUG:", fmt.Sprint(args...)) }
func (*MockLogger) Debugf(format string, args ...any) { fmt.Printf("DEBUG: "+format+"\n", args...) }
func (*MockLogger) Debugw(msg string, kv ...any)      { fmt.Println("DEBUG:", msg, keyValues(kv)) }

func (m *MockLogger) DebugfContext(_ context.Context, format string, args ...any) {
	m.Debugf(format, args...)
}

func (m *MockLogger) DebugwContext(_ context.Context, msg string, kv ...any) { m.Debugw(msg, kv...) }

func (*MockLogger) Info(args ...any)                 { fmt.Println("INFO:", fmt.Sprint(args...)) }
func (*MockLogger) Infof(format string, args ...any) { fmt.Printf("INFO: "+format+"\n", args...) }
func (*MockLogger) Infow(msg string, kv ...any)      { fmt.Println("INFO:", msg, keyValues(kv)) }

func (m *MockLogger) InfofContext(_ context.Context, format string, args ...any) {
	m.Infof(format, args...)
}
func (m *MockLogger) InfowContext(_ context.Context, msg string, kv ...any) { m.Infow(msg, kv...) }

func (*MockLogger) Warn(args ...any)                 { fmt.Println("WARN:", fmt.Sprint(args...)) }
func (*MockLogger) Warnf(format string, args ...any) { fmt.Printf("WARN: "+format+"\n", args...) }
func (*MockLogger) Warnw(msg string, kv ...any)      { fmt.Println("WARN:", msg, keyValues(kv)) }

func (m *MockLogger) WarnfContext(_ context.Context, format string, args ...any) {
	m.Warnf(format, args...)
}
func (m *MockLogger) WarnwContext(_ context.Context, msg string, kv ...any) { m.Warnw(msg, kv...) }

func (*MockLogger) Error(args ...any)                 { fmt.Println("ERROR:", fmt.Sprint(args...)) }
func (*MockLogger) Errorf(format string, args ...any) { fmt.Printf("ERROR: "+format+"\n", args...) }
func (*MockLogger) Errorw(msg string, kv ...any)      { fmt.Println("ERROR:", msg, keyValues(kv)) }

func (m *MockLogger) ErrorfContext(_ context.Context, format string, args ...any) {
	m.Errorf(format, args...)
}

func (m *MockLogger) ErrorwContext(_ context.Context, msg string, kv ...any) { m.Errorw(msg, kv...) }

// keyValues formats alternating keys and values as k=v pairs.
func keyValues(kv []any) string {
	var b strings.Builder
	for i := 0; i < len(kv); i += 2 {
		if i > 0 {
			b.WriteByte(' ')
		}
		if i+1 < len(kv) {
			fmt.Fprintf(&b, "%v=%v", kv[i], kv[i+1])
		} else {
			fmt.Fprintf(&b, "%v", kv[i])
		}
	}
	return b.String()
}
