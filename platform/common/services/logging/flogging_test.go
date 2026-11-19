/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/grpclog"
)

// TestSetHandler_Flogging checks a line of a library that logs through flogging reaches
// the application handler, with its level, caller, logger name and fields, and nothing
// reaches flogging's writer; and that SetHandler(nil) sends such lines to the writer again.
func TestSetHandler_Flogging(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	app, out := appHandler(t, "info", slog.LevelDebug)
	before := len(out.lines(t))

	flogging.MustGetLogger("x").Warnw("from flogging", "k", "v", "err", errors.New("boom"))
	grpclog.Info("from grpc")
	SetHandler(nil)
	flogging.MustGetLogger("x").Infof("to the writer")

	lines := app.lines(t)
	require.Len(t, lines, 2)
	assert.Equal(t, "from flogging", lines[0]["msg"])
	assert.Equal(t, "WARN", lines[0]["level"])
	assert.Equal(t, "x", lines[0][loggerAttr])
	assert.Equal(t, "v", lines[0]["k"])
	assert.Equal(t, "boom", lines[0]["err"])
	assert.True(t, strings.HasSuffix(lines[0]["source"].(map[string]any)["function"].(string), ".TestSetHandler_Flogging"), lines[0])
	assert.Equal(t, "from grpc", lines[1]["msg"])
	assert.Equal(t, "grpc", lines[1][loggerAttr])
	written := out.lines(t)[before:]
	require.Len(t, written, 1)
	assert.Equal(t, "to the writer", written[0]["msg"])
}

// TestSetHandler_FloggingFilters checks a flogging line reaches the handler only if both
// logging.spec and the handler enable it.
func TestSetHandler_FloggingFilters(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	app, _ := appHandler(t, "x=warn:debug", slog.LevelInfo)

	flogging.MustGetLogger("x").Info("dropped by logging.spec")
	flogging.MustGetLogger("y").Debug("dropped by the handler")
	flogging.MustGetLogger("y").Info("written")

	lines := app.lines(t)
	require.Len(t, lines, 1)
	assert.Equal(t, "written", lines[0]["msg"])
}

// TestSetHandler_FloggingWith pins a limitation: flogging keeps the fields of With in its
// encoders, so the handler doesn't get them. A library that starts to rely on With
// makes this test fail.
func TestSetHandler_FloggingWith(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	app, _ := appHandler(t, "info", slog.LevelDebug)

	flogging.MustGetLogger("y").With("k", "v").Info("without k")

	lines := app.lines(t)
	require.Len(t, lines, 1)
	assert.NotContains(t, lines[0], "k")
}

// TestSetHandler_FloggingInitWriter checks Init with a writer while a handler is set
// doesn't send flogging lines to that writer, and that SetHandler(nil) does.
func TestSetHandler_FloggingInitWriter(t *testing.T) { //nolint:paralleltest // changes the global logging configuration
	app, _ := appHandler(t, "info", slog.LevelDebug)
	var w bytes.Buffer
	require.NoError(t, Init(Config{Format: "%{message}", LogSpec: "info", Writer: &w}))

	flogging.MustGetLogger("x").Info("to the handler")
	require.Empty(t, w.String())
	SetHandler(nil)
	flogging.MustGetLogger("x").Info("to the writer")

	assert.Equal(t, "to the writer\n", w.String())
	assert.Len(t, app.lines(t), 2) // the line Init logs, and the first one
}
