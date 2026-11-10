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

type (
	Recorder = floggingtest.Recorder
	Option   = floggingtest.Option
)

// NewTestLogger returns a Logger that records what is written to it, so tests
// can assert on log output. It supports every Logger method, including the
// *Context ones, and its entries, like those of loggers derived with Named or
// With, report the caller of the log method.
func NewTestLogger(tb testing.TB, options ...Option) (Logger, *Recorder) {
	tb.Helper()
	l, r := floggingtest.NewTestLogger(tb, options...)
	// l.Zap() carries the caller skip of the FabricLogger floggingtest built;
	// remove it, since newLogger adds its own.
	return newLogger(l.Zap().WithOptions(zap.AddCallerSkip(-1))), r
}
