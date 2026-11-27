/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/logging"
)

// TestMustGetLoggerOtherPackage checks a logger is named after the package of the caller of
// MustGetLogger, here an external test package, with its params appended. The %{module}
// verb prints the logger name.
func TestMustGetLoggerOtherPackage(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	var buf bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{Format: "%{module}", LogSpec: "info", Writer: &buf}))
	t.Cleanup(func() { require.NoError(t, logging.Init(logging.Config{Writer: os.Stderr})) })
	buf.Reset() // drop what Init itself logs

	logging.MustGetLogger().Info("x")
	logging.MustGetLogger("some", "thing").Info("x")

	require.Equal(t,
		"fsc.pkg.logging_test\n"+
			"fsc.pkg.logging_test.some.thing\n",
		buf.String())
}
