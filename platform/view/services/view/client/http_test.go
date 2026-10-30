/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// A server of a newer version may answer with fields this version does not know; the client
// ignores them, as gRPC does.
func TestHTTPTransportIgnoresUnknownResponseFields(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"response":"cmVz","futureField":7}`))
	}))
	t.Cleanup(ts.Close)
	tr := &httpTransport{url: ts.URL, client: ts.Client(), maxRecvMsgSize: 1024}

	scr, err := tr.ProcessCommand(t.Context(), &protos.SignedCommand{Command: []byte("cmd")})
	require.NoError(t, err)
	require.Equal(t, "res", string(scr.Response))
}
