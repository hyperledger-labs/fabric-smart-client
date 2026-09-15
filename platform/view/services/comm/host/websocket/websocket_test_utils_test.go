/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
)

// Each of NewConfigFromProperties's file reads propagates its own error instead of panicking.
func TestNewConfigFromPropertiesPropagatesReadErrors(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM, err := websocket.GenerateTestCert("node")
	require.NoError(t, err)
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))
	missing := filepath.Join(dir, "missing")

	for _, tc := range []struct {
		name, key, cert      string
		serverCAs, clientCAs []string
		wantErr              string
	}{
		{name: "bad cert", key: keyFile, cert: missing, wantErr: "failed to read cert"},
		{name: "bad key", key: missing, cert: certFile, wantErr: "failed to read private key"},
		{name: "bad server root CA", key: keyFile, cert: certFile, serverCAs: []string{missing}, wantErr: "failed to read server root CAs"},
		{name: "bad client root CA", key: keyFile, cert: certFile, clientCAs: []string{missing}, wantErr: "failed to read client root CAs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := websocket.NewConfigFromProperties("127.0.0.1:0", tc.key, tc.cert, tc.serverCAs, tc.clientCAs, false, 100, nil)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
