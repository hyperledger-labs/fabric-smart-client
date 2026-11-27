/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"bytes"
	"log/slog"
	"os"
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/libp2p/go-libp2p/gologshim"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/logging"
)

// The tests log through gologshim.Logger, as every libp2p package does, for
// example basichost in p2p/host/basic/basic_host.go.

// restoreLogging puts back the process-wide logging configuration TestMain sets.
func restoreLogging(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		logging.SetHandler(nil)
		require.NoError(t, logging.Init(logging.Config{LogSpec: testLogSpec(), Writer: os.Stderr}))
	})
}

func TestLibp2pLogsUseFSCFormat(t *testing.T) { //nolint:paralleltest // changes process-wide logging
	var buf bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{Format: "json", LogSpec: "libp2p=info", Writer: &buf}))
	restoreLogging(t)

	gologshim.Logger("basichost").Info("fsc-libp2p-line", "k", "v")

	out := buf.String()
	require.Contains(t, out, "fsc-libp2p-line")
	require.Contains(t, out, `"name":"libp2p"`)
	require.Contains(t, out, `"subsystem":"basichost"`)
	require.Contains(t, out, `"k":"v"`)
	require.Contains(t, out, `"caller":"libp2p/logbridge_test.go:`)
}

// TestLibp2pLogsFollowSpecAsWritten checks that a spec without a libp2p term applies
// to libp2p like to any other logger: debug writes libp2p's Debug lines.
func TestLibp2pLogsFollowSpecAsWritten(t *testing.T) { //nolint:paralleltest // changes process-wide logging
	var buf bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{Format: "%{message}", LogSpec: "debug", Writer: &buf}))
	restoreLogging(t)

	gologshim.Logger("basichost").Debug("fsc-libp2p-debug")
	require.Contains(t, buf.String(), "fsc-libp2p-debug")
}

func TestLibp2pLogsFollowLoggingSpec(t *testing.T) { //nolint:paralleltest // changes process-wide logging
	var buf bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{Format: "%{message}", LogSpec: "libp2p=error:info", Writer: &buf}))
	restoreLogging(t)

	log := gologshim.Logger("basichost")
	log.Warn("fsc-libp2p-dropped-warn")
	log.Error("fsc-libp2p-kept-error")
	out := buf.String()
	require.NotContains(t, out, "fsc-libp2p-dropped-warn")
	require.Contains(t, out, "fsc-libp2p-kept-error")

	// A spec changed at runtime, as by PUT /logspec, applies to a logger already in use.
	require.NoError(t, flogging.Global.ActivateSpec("libp2p=info:info"))
	log.Info("fsc-libp2p-kept-info")
	require.Contains(t, buf.String(), "fsc-libp2p-kept-info")
}

func TestLibp2pLogsReachApplicationHandler(t *testing.T) { //nolint:paralleltest // changes process-wide logging
	var app, out bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{Format: "json", LogSpec: "libp2p=info", Writer: &out}))
	restoreLogging(t)
	logging.SetHandler(slog.NewJSONHandler(&app, &slog.HandlerOptions{Level: slog.LevelDebug}))

	gologshim.Logger("basichost").Info("fsc-libp2p-via-handler")

	got := app.String()
	require.Contains(t, got, "fsc-libp2p-via-handler")
	require.Contains(t, got, `"logger":"libp2p"`)
	require.Contains(t, got, `"subsystem":"basichost"`)
	require.NotContains(t, got, `"logger":"basichost"`)
	require.NotContains(t, out.String(), "fsc-libp2p-via-handler")
}
