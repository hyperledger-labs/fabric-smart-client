/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package docker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	dcli "github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// fakeAPIClient embeds the (nil) interface so it satisfies dcli.APIClient
// without implementing every one of its methods; only ContainerLogs is
// overridden, matching what StartLogs actually calls.
type fakeAPIClient struct {
	dcli.APIClient
	logs    dcli.ContainerLogsResult
	logsErr error
}

func (f *fakeAPIClient) ContainerLogs(context.Context, string, dcli.ContainerLogsOptions) (dcli.ContainerLogsResult, error) {
	return f.logs, f.logsErr
}

type closeTrackingReader struct {
	io.Reader
	closed chan struct{}
}

func (c *closeTrackingReader) Close() error {
	close(c.closed)
	return nil
}

func TestStartLogsReturnsContainerLogsErrorSynchronously(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	cli := &fakeAPIClient{logsErr: wantErr}

	err := StartLogs(cli, "container-id", "container-name")

	require.ErrorIs(t, err, wantErr)
}

func TestStartLogsScansUntilEOFAndClosesReader(t *testing.T) {
	t.Parallel()

	closed := make(chan struct{})
	cli := &fakeAPIClient{
		logs: &closeTrackingReader{
			Reader: strings.NewReader("line one\nline two\n"),
			closed: closed,
		},
	}

	err := StartLogs(cli, "container-id", "container-name")
	require.NoError(t, err)

	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("StartLogs did not close the log reader after reaching EOF")
	}
}
