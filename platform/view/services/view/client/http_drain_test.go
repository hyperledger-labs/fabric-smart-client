/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/web"
)

// statusStream is a view service whose StreamCommand ends with err.
type statusStream struct {
	protos.UnimplementedViewServiceServer
	err error
}

func (s statusStream) StreamCommand(protos.ViewService_StreamCommandServer) error { return s.err }

// A stream a draining node refuses, or whose view its shutdown cancelled, reaches the client as
// the gRPC status the gRPC transport would report, so the client can tell "retry elsewhere" from
// "check the outcome". Other server errors keep their WebSocket close error.
func TestWSCommandClientMapsDrainStatuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
		want codes.Code
	}{
		{"draining", status.Error(codes.Unavailable, "node is draining"), codes.Unavailable},
		{"cancelled by shutdown", status.Error(codes.Aborted, protos.AbortedByShutdownMessage), codes.Aborted},
		{"other error", status.Error(codes.Internal, "boom"), codes.Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts := httptest.NewServer(web.NewStreamCommandHandler(statusStream{err: tc.err}, 1024))
			t.Cleanup(ts.Close)
			tr := &httpTransport{wsURL: "ws" + strings.TrimPrefix(ts.URL, "http"), client: ts.Client(), maxRecvMsgSize: 1024}

			scc, err := tr.StreamCommand(t.Context())
			require.NoError(t, err)
			_, err = scc.Recv()
			require.Error(t, err)
			require.Equal(t, tc.want, status.Code(err), "%v", err)
			_ = scc.CloseSend()
		})
	}
}
