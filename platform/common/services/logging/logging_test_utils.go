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
// them; see docs/agents/testing.md.

type (
	Recorder = floggingtest.Recorder
	Option   = floggingtest.Option
)

// Named returns an Option that names the logger under test.
func Named(loggerName string) Option {
	return func(r *floggingtest.RecordingCore, l *zap.Logger) *zap.Logger {
		return l.Named(loggerName)
	}
}

// NewTestLogger returns a Logger that records what is written to it, so tests
// can assert on log output.
func NewTestLogger(tb testing.TB, options ...Option) (Logger, *Recorder) {
	tb.Helper()
	l, r := floggingtest.NewTestLogger(tb, options...)
	return &logger{fabricLogger: l}, r
}
