/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/wsproto"
)

func TestProcessCommandHandlerDrainStatuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		code codes.Code
		want int
	}{
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.Aborted, http.StatusConflict},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			t.Parallel()
			ts := newTestServer(t, &echoService{processErr: status.Error(tc.code, "node is draining")}, 1024)
			body, _ := protojson.Marshal(&protos.SignedCommand{Command: []byte("cmd")})
			code, raw := post(t, ts.URL+protos.ProcessCommandHTTPPath, body)
			require.Equal(t, tc.want, code)
			require.Contains(t, string(raw), "node is draining")
		})
	}
}

func TestStreamCommandHandlerUnavailableClosesTryAgainLater(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, &echoService{streamErr: status.Error(codes.Unavailable, "node is draining")}, 1024)
	conn := dial(t, ts)
	require.NoError(t, wsproto.Write(conn, &protos.SignedCommand{Command: []byte("cmd")}))
	require.NoError(t, wsproto.Write(conn, &protos.CallViewResponse{Result: []byte("done")}))
	_, _, err := conn.ReadMessage()
	require.True(t, websocket.IsCloseError(err, websocket.CloseTryAgainLater), "got %v", err)
}
