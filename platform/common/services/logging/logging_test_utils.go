/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging/floggingtest"
	"go.uber.org/zap"
)

// Test helpers for packages that need to assert on log output. They live in a
// *_test_utils.go file rather than a _test.go one because other packages import
// them; see docs/dev/testing.md.

// Recorder holds what a logger from [NewTestLogger] wrote.
type Recorder = floggingtest.Recorder

// NewTestLogger returns a Logger that records what is written to it at every level, so
// tests can assert on log output. Its entries, like those of loggers derived with Named
// or With, report the caller of the log method.
func NewTestLogger(tb testing.TB) (Logger, *Recorder) {
	tb.Helper()
	var zl *zap.Logger
	// The option sees the zap logger floggingtest built, before it is wrapped in a
	// FabricLogger and its caller skip.
	_, r := floggingtest.NewTestLogger(tb, func(_ *floggingtest.RecordingCore, l *zap.Logger) *zap.Logger {
		zl = l
		return l
	})
	return newLogger(zl.WithOptions(zap.AddCaller())), r
}
