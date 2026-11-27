/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	dcli "github.com/moby/moby/client"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// fakeLogsClient embeds the (nil) interface so it satisfies dcli.APIClient; only
// ContainerLogs, which startContainerLogger calls, is overridden.
type fakeLogsClient struct {
	dcli.APIClient
	logs    string
	logsErr error
}

func (f *fakeLogsClient) ContainerLogs(context.Context, string, dcli.ContainerLogsOptions) (dcli.ContainerLogsResult, error) {
	if f.logsErr != nil {
		return nil, f.logsErr
	}
	return io.NopCloser(strings.NewReader(f.logs)), nil
}

// lineLogger sends every line logged to it to lines.
type lineLogger struct{ lines chan string }

func (l lineLogger) Debugf(format string, args ...any) {
	l.lines <- "DEBUG " + fmt.Sprintf(format, args...)
}

func (l lineLogger) Errorf(format string, args ...any) {
	l.lines <- "ERROR " + fmt.Sprintf(format, args...)
}

func (l lineLogger) next(t *testing.T) string {
	t.Helper()
	select {
	case line := <-l.lines:
		return line
	case <-time.After(2 * time.Second):
		t.Fatal("no line logged")
		return ""
	}
}

// frame returns payload as one frame of a multiplexed stream: an 8-byte header holding
// the stream (2 is stderr) and the payload length as a big-endian uint32.
func frame(payload string) string {
	header := make([]byte, 8)
	header[0] = 2
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	return string(header) + payload
}

func TestStartContainerLoggerDemultiplexes(t *testing.T) {
	t.Parallel()
	l := lineLogger{lines: make(chan string, 10)}

	startContainerLogger(t.Context(), &fakeLogsClient{logs: frame("ready to accept ") + frame("100%d connections\nnext\n")}, "id", l)

	require.Equal(t, "DEBUG ready to accept 100%d connections", l.next(t))
	require.Equal(t, "DEBUG next", l.next(t))
}

func TestStartContainerLoggerLogsContainerLogsError(t *testing.T) {
	t.Parallel()
	l := lineLogger{lines: make(chan string, 10)}

	startContainerLogger(t.Context(), &fakeLogsClient{logsErr: errors.New("boom")}, "id", l)

	require.Equal(t, "ERROR can't show logs for container id: boom", l.next(t))
	// The goroutine returns instead of using the nil reader; a panic would end the test binary.
	select {
	case line := <-l.lines:
		t.Fatalf("unexpected line %q", line)
	case <-time.After(100 * time.Millisecond):
	}
}
