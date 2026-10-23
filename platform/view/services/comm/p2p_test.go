/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

type failingReader struct{ err error }

func (r failingReader) ReadMsg(proto.Message) error { return r.err }
func (failingReader) Close() error                  { return nil }

func TestHandleIncoming_ReadErrorCounted(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		err      error
		stopping bool
		closed   bool
		want     int
	}{
		{name: "running", err: errors.New("read failed"), want: 1},
		{name: "stopping", err: errors.New("read failed"), stopping: true, want: 0},
		{name: "clean EOF", err: errors.Wrap(io.EOF, "failed reading data"), want: 0},
		{name: "truncated message", err: io.ErrUnexpectedEOF, want: 1},
		{name: "already closed", err: errors.New("read failed"), closed: true, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, err := NewNode(context.Background(), &mockHost{}, &disabled.Provider{})
			require.NoError(t, err)
			readErrors := &mock.Counter{}
			p.m.StreamReadErrors = readErrors
			p.isStopping = tc.stopping

			sh := &streamHandler{stream: &mockStream{ctx: t.Context()}, reader: failingReader{err: tc.err}, node: p}
			sh.closed.Store(tc.closed)
			p.handlersWg.Add(1)
			sh.handleIncoming()

			assert.Equal(t, tc.want, readErrors.AddCallCount())
			for i := range readErrors.AddCallCount() {
				assert.InDelta(t, 1.0, readErrors.AddArgsForCall(i), 0)
			}
		})
	}
}
