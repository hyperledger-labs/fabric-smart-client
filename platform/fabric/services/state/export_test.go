/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// recordLogs replaces the package logger for t and returns the recorder.
func recordLogs(t *testing.T) *logging.Recorder {
	t.Helper()
	l, r := logging.NewTestLogger(t)
	old := logger
	logger = l
	t.Cleanup(func() { logger = old })
	return r
}
