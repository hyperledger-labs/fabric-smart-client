/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package migrate

import (
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// SetLockIntervals shortens the Postgres lock polling and logging intervals for t.
func SetLockIntervals(t *testing.T, poll, log time.Duration) {
	t.Helper()
	oldPoll, oldLog := lockPollInterval, lockLogInterval
	lockPollInterval, lockLogInterval = poll, log
	t.Cleanup(func() { lockPollInterval, lockLogInterval = oldPoll, oldLog })
}

// RecordLogs replaces the package logger for t and returns the recorder.
func RecordLogs(t *testing.T) *logging.Recorder {
	t.Helper()
	l, r := logging.NewTestLogger(t)
	old := logger
	logger = l
	t.Cleanup(func() { logger = old })
	return r
}

// LockKey is lockKey.
var LockKey = lockKey
