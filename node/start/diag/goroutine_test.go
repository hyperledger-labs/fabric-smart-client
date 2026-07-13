/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package diag_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/node/start/diag"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

func TestCaptureGoRoutines(t *testing.T) {
	t.Parallel()
	output, err := diag.CaptureGoRoutines()
	require.NoError(t, err)

	require.Regexp(t, `goroutine \d+ \[running\]:`, output)
	require.Contains(t, output, "github.com/hyperledger-labs/fabric-smart-client/node/start/diag.CaptureGoRoutines")
}

func TestLogGoRoutines(t *testing.T) {
	t.Parallel()
	logger, recorder := logging.NewTestLogger(t, logging.Named("goroutine"))
	diag.LogGoRoutines(logger)

	require.NotEmpty(t, recorder.EntriesMatching(`goroutine \d+ \[running\]:`))
}
