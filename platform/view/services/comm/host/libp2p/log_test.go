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
	golog "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// restoreLogging puts back the process-wide logging configuration TestMain sets.
func restoreLogging(t *testing.T) {
	t.Helper()
	spec := os.Getenv("FABRIC_LOGGING_SPEC")
	if len(spec) == 0 {
		spec = os.Getenv("FSC_LOGSPEC")
	}
	if len(spec) == 0 {
		spec = "error"
	}
	t.Cleanup(func() {
		logging.SetHandler(nil)
		require.NoError(t, logging.Init(logging.Config{LogSpec: spec, Writer: os.Stderr}))
	})
}

func newLoggingHost(t *testing.T) *host {
	t.Helper()
	cfg := &mock.LibP2PConfig{}
	cfg.ListenAddressReturns(freeLibP2PAddresses(t, 1)[0])
	sk, _ := generateKey(t)
	h, err := newLibP2PHost(cfg, sk, newMetrics(&disabled.Provider{}))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	return h
}

func TestLibp2pLogsUseFSCFormat(t *testing.T) { //nolint:paralleltest // changes process-wide logging and go-log
	var buf bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{Format: "json", LogSpec: "debug", Writer: &buf}))
	restoreLogging(t)
	newLoggingHost(t)

	golog.Logger("basichost").Infow("fsc-libp2p-line", "k", "v")

	out := buf.String()
	require.Contains(t, out, "fsc-libp2p-line")
	require.Contains(t, out, `"name":"libp2p"`)
	require.Contains(t, out, `"subsystem":"basichost"`)
	require.Contains(t, out, `"k":"v"`)
}

func TestLibp2pLogsFollowLoggingSpec(t *testing.T) { //nolint:paralleltest // changes process-wide logging and go-log
	var buf bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{Format: "%{message}", LogSpec: "libp2p=warn:info", Writer: &buf}))
	restoreLogging(t)
	newLoggingHost(t)

	golog.Logger("basichost").Info("fsc-libp2p-dropped-info")
	golog.Logger("basichost").Warn("fsc-libp2p-kept-warn")
	out := buf.String()
	require.NotContains(t, out, "fsc-libp2p-dropped-info")
	require.Contains(t, out, "fsc-libp2p-kept-warn")

	require.NoError(t, flogging.Global.ActivateSpec("libp2p=info:info"))
	golog.Logger("basichost").Info("fsc-libp2p-kept-info")
	require.Contains(t, buf.String(), "fsc-libp2p-kept-info")
}

func TestLibp2pLogsReachApplicationHandler(t *testing.T) { //nolint:paralleltest // changes process-wide logging and go-log
	var app, out bytes.Buffer
	require.NoError(t, logging.Init(logging.Config{Format: "json", LogSpec: "debug", Writer: &out}))
	restoreLogging(t)
	logging.SetHandler(slog.NewJSONHandler(&app, &slog.HandlerOptions{Level: slog.LevelDebug}))
	newLoggingHost(t)

	golog.Logger("basichost").Info("fsc-libp2p-via-handler")

	got := app.String()
	require.Contains(t, got, "fsc-libp2p-via-handler")
	require.Contains(t, got, `"logger":"libp2p"`)
	require.Contains(t, got, `"subsystem":"basichost"`)
	require.NotContains(t, out.String(), "fsc-libp2p-via-handler")
}
