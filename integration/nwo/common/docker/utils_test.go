/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	dcli "github.com/moby/moby/client"
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
)

// fakeAPIClient embeds the (nil) interface so it satisfies dcli.APIClient
// without implementing every one of its methods; only ContainerInspect and
// ContainerLogs are overridden, matching what StartLogs actually calls.
type fakeAPIClient struct {
	dcli.APIClient
	tty        bool
	inspectErr error
	logs       dcli.ContainerLogsResult
	logsErr    error
}

func (f *fakeAPIClient) ContainerInspect(context.Context, string, dcli.ContainerInspectOptions) (dcli.ContainerInspectResult, error) {
	return dcli.ContainerInspectResult{Container: container.InspectResponse{Config: &container.Config{Tty: f.tty}}}, f.inspectErr
}

func (f *fakeAPIClient) ContainerLogs(context.Context, string, dcli.ContainerLogsOptions) (dcli.ContainerLogsResult, error) {
	return f.logs, f.logsErr
}

type closeTrackingReader struct {
	io.Reader
	closed chan struct{}
}

func newCloseTrackingReader(s string) *closeTrackingReader {
	return &closeTrackingReader{Reader: strings.NewReader(s), closed: make(chan struct{})}
}

func (c *closeTrackingReader) Close() error {
	close(c.closed)
	return nil
}

// frame returns payload as one frame of a multiplexed stream: an 8-byte header holding
// the stream (1 stdout, 2 stderr) and the payload length as a big-endian uint32.
func frame(stream byte, payload string) string {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	return string(header) + payload
}

// teeGinkgoWriter returns a buffer that receives a copy of what is written to
// GinkgoWriter until the test ends. Tests that use it must not run in parallel.
func teeGinkgoWriter(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	ginkgo.GinkgoWriter.TeeTo(&buf)
	t.Cleanup(ginkgo.GinkgoWriter.ClearTeeWriters)
	return &buf
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s was not closed", what)
	}
}

func TestForwardLogs(t *testing.T) { //nolint:paralleltest // tees the global GinkgoWriter
	for _, tc := range []struct { //nolint:paralleltest // tees the global GinkgoWriter
		name  string
		tty   bool
		input string
		want  string
	}{
		{
			name:  "multiplexed",
			input: frame(1, "out one\nout ") + frame(2, "two\n") + frame(2, "err 100%d\n"),
			want:  "[c1] out one\n[c1] out two\n[c1] err 100%d\n",
		},
		{
			name:  "tty",
			tty:   true,
			input: "line one\r\nline two\n",
			want:  "[c1] line one\r\n[c1] line two\n",
		},
		{
			name:  "tty stream read as multiplexed",
			input: "line one\n",
			want:  "[c1] failed forwarding logs: unrecognized stream: 108\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := teeGinkgoWriter(t)
			r := newCloseTrackingReader(tc.input)

			waitClosed(t, ForwardLogs("c1", r, tc.tty), "the returned channel")

			waitClosed(t, r.closed, "the reader")
			require.Equal(t, tc.want, buf.String())
		})
	}
}

func TestStartLogsReturnsErrorsSynchronously(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	for name, cli := range map[string]*fakeAPIClient{
		"inspect": {inspectErr: wantErr},
		"logs":    {logsErr: wantErr},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, StartLogs(cli, "container-id", "container-name"), wantErr)
		})
	}
}

func TestStartLogsDemultiplexesWithoutTTY(t *testing.T) { //nolint:paralleltest // tees the global GinkgoWriter
	buf := teeGinkgoWriter(t)
	r := newCloseTrackingReader(frame(1, "line one\n") + frame(2, "line two\n"))

	require.NoError(t, StartLogs(&fakeAPIClient{logs: r}, "container-id", "c2"))

	// The reader is closed once the copy has ended.
	waitClosed(t, r.closed, "the reader")
	require.Equal(t, "[c2] line one\n[c2] line two\n", buf.String())
}
