/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc/tlsgen"
	"github.com/stretchr/testify/require"
)

func TestTLSConfigVersions(t *testing.T) {
	t.Parallel()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)

	serverKP, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

	clientKP, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)

	dir := t.TempDir()
	serverCertFile := filepath.Join(dir, "server-cert.pem")
	serverKeyFile := filepath.Join(dir, "server-key.pem")
	clientCertFile := filepath.Join(dir, "client-cert.pem")
	clientKeyFile := filepath.Join(dir, "client-key.pem")

	require.NoError(t, os.WriteFile(serverCertFile, serverKP.Cert, 0o600))
	require.NoError(t, os.WriteFile(serverKeyFile, serverKP.Key, 0o600))
	require.NoError(t, os.WriteFile(clientCertFile, clientKP.Cert, 0o600))
	require.NoError(t, os.WriteFile(clientKeyFile, clientKP.Key, 0o600))

	certPool := x509.NewCertPool()
	require.True(t, certPool.AppendCertsFromPEM(ca.CertBytes()))

	clientTLS, err := newClientTLSConfig(certPool, clientKeyFile, clientCertFile, nil)
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS12), clientTLS.MinVersion)
	require.Equal(t, uint16(tls.VersionTLS13), clientTLS.MaxVersion)

	serverTLS, err := newServerTLSConfig(certPool, serverKeyFile, serverCertFile, true, nil)
	require.NoError(t, err)
	require.Equal(t, uint16(tls.VersionTLS12), serverTLS.MinVersion)
	require.Equal(t, uint16(tls.VersionTLS13), serverTLS.MaxVersion)
}
