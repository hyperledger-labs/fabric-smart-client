/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path"
	"testing"
	"time"

	queryservice2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/queryservice"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func createTestCaCert(t *testing.T) string {
	t.Helper()

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(2025),
		Subject:               pkix.Name{},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, 1),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	require.NoError(t, err)

	caPath := path.Join(t.TempDir(), "ca.crt")
	err = os.WriteFile(caPath, caPEM.Bytes(), 0o644)
	require.NoError(t, err)

	return caPath
}

func TestGRPC(t *testing.T) {
	tmpCertPath := createTestCaCert(t)

	table := []struct {
		name   string
		cfg    map[string]any
		checks func(t *testing.T, client *grpc.ClientConn, err error)
	}{
		{
			name: "no endpoints in config",
			cfg: map[string]any{
				"queryService.queryTimeout": 10 * time.Second,
			},
			checks: func(t *testing.T, client *grpc.ClientConn, err error) {
				t.Helper()
				require.Nil(t, client)
				require.Error(t, err)
			},
		},
		{
			name: "too many endpoints",
			cfg: map[string]any{
				"queryService.queryTimeout": 10 * time.Second,
				"queryService.Endpoints": []any{
					map[string]any{
						"address": "localhost:9988",
					},
					map[string]any{
						"address": "localhost:9999",
					},
				},
			},
			checks: func(t *testing.T, client *grpc.ClientConn, err error) {
				t.Helper()
				require.Nil(t, client)
				require.Error(t, err)
			},
		},
		{
			name: "invalid endpoint address",
			cfg: map[string]any{
				"queryService.queryTimeout": 10 * time.Second,
				"queryService.Endpoints": []any{
					map[string]any{
						"address": "",
					},
				},
			},
			checks: func(t *testing.T, client *grpc.ClientConn, err error) {
				t.Helper()
				require.Nil(t, client)
				require.ErrorIs(t, err, queryservice2.ErrInvalidAddress)
			},
		},
		{
			name: "with connection timeout",
			cfg: map[string]any{
				"queryService.queryTimeout": 10 * time.Second,
				"queryService.Endpoints": []any{
					map[string]any{
						"address":           "localhost:8899",
						"connectionTimeout": 0 * time.Second,
					},
				},
			},
			checks: func(t *testing.T, client *grpc.ClientConn, err error) {
				t.Helper()
				require.NotNil(t, client)
				require.NoError(t, err)
			},
		},
		{
			name: "with TLS enabled",
			cfg: map[string]any{
				"queryService.queryTimeout": 10 * time.Second,
				"queryService.Endpoints": []any{
					map[string]any{
						"address":         "localhost:8899",
						"tlsEnabled":      true,
						"tlsRootCertFile": tmpCertPath,
					},
				},
			},
			checks: func(t *testing.T, client *grpc.ClientConn, err error) {
				t.Helper()
				require.NotNil(t, client)
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range table {
		t.Run(fmt.Sprintf("grpcClient %v", tc.name), func(t *testing.T) {
			t.Parallel()
			cs := newConfigService(tc.cfg)
			c, err := queryservice2.NewConfig(cs)
			require.NoError(t, err)
			client, err := queryservice2.GrpcClient(c)
			tc.checks(t, client, err)
		})
	}
}
