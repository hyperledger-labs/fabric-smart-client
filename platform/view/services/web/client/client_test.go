/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

// --- Config URL generation ---

// Verifies that WebURL returns "http" when no TLS is configured,
// and "https" when TLS is enabled via either CACertPath or CACertRaw.
func TestConfig_WebURL_Selects_Correct_Protocol(t *testing.T) {
	t.Parallel()

	table := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name:     "Returns_HTTP_When_No_TLS_Configured",
			config:   Config{Host: "localhost:8080"},
			expected: "http://localhost:8080",
		},
		{
			name:     "Returns_HTTPS_When_CACertPath_Set",
			config:   Config{Host: "localhost:8443", CACertPath: "/path/to/ca.pem"},
			expected: "https://localhost:8443",
		},
		{
			name:     "Returns_HTTPS_When_Only_CACertRaw_Set",
			config:   Config{Host: "localhost:8443", CACertRaw: []byte("cert-data")},
			expected: "https://localhost:8443",
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, tc.config.WebURL())
		})
	}
}

// Same as WebURL but for the WebSocket protocol: "ws" vs "wss".
func TestConfig_WsURL_Selects_Correct_Protocol(t *testing.T) {
	t.Parallel()

	table := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name:     "Returns_WS_When_No_TLS_Configured",
			config:   Config{Host: "localhost:8080"},
			expected: "ws://localhost:8080",
		},
		{
			name:     "Returns_WSS_When_CACertPath_Set",
			config:   Config{Host: "localhost:8443", CACertPath: "/path/to/ca.pem"},
			expected: "wss://localhost:8443",
		},
		{
			name:     "Returns_WSS_When_Only_CACertRaw_Set",
			config:   Config{Host: "localhost:8443", CACertRaw: []byte("cert-data")},
			expected: "wss://localhost:8443",
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, tc.config.WsURL())
		})
	}
}

// --- NewClient construction ---

// Verifies that NewClient correctly configures the internal http.Client,
// URLs, and TLS settings across all supported config combinations:
// no TLS, CA from raw bytes, CA from file path, and mutual TLS with client certs.
func TestNewClient_Success_Configures_Client_Correctly(t *testing.T) {
	t.Parallel()

	caCertPEM, caKeyPEM := generateSelfSignedCert(t)
	caPath := writeTempFile(t, "ca-cert.pem", caCertPEM)
	certPath := writeTempFile(t, "tls-cert.pem", caCertPEM)
	keyPath := writeTempFile(t, "tls-key.pem", caKeyPEM)

	table := []struct {
		name                string
		config              Config
		expectTLS           bool
		expectURL           string
		expectWsURL         string
		expectClientCertLen int
	}{
		{
			name:        "No_TLS_Uses_Plain_HTTP_And_WS",
			config:      Config{Host: "localhost:8080"},
			expectTLS:   false,
			expectURL:   "http://localhost:8080",
			expectWsURL: "ws://localhost:8080",
		},
		{
			name:        "CACertRaw_Enables_TLS_With_HTTPS_And_WSS",
			config:      Config{Host: "localhost:8443", CACertRaw: caCertPEM},
			expectTLS:   true,
			expectURL:   "https://localhost:8443",
			expectWsURL: "wss://localhost:8443",
		},
		{
			name:        "CACertPath_Enables_TLS_With_HTTPS_And_WSS",
			config:      Config{Host: "localhost:8443", CACertPath: caPath},
			expectTLS:   true,
			expectURL:   "https://localhost:8443",
			expectWsURL: "wss://localhost:8443",
		},
		{
			name: "Mutual_TLS_Loads_Client_Certificates",
			config: Config{
				Host:        "localhost:8443",
				CACertPath:  caPath,
				TLSCertPath: certPath,
				TLSKeyPath:  keyPath,
			},
			expectTLS:           true,
			expectURL:           "https://localhost:8443",
			expectWsURL:         "wss://localhost:8443",
			expectClientCertLen: 1,
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(&tc.config)

			require.NoError(t, err)
			require.NotNil(t, client)
			require.Equal(t, tc.expectURL, client.url)
			require.Equal(t, tc.expectWsURL, client.wsUrl)

			if tc.expectTLS {
				require.NotNil(t, client.tlsConfig)
				require.NotNil(t, client.tlsConfig.RootCAs)
				if tc.expectClientCertLen > 0 {
					require.Len(t, client.tlsConfig.Certificates, tc.expectClientCertLen)
				}
			} else {
				require.Nil(t, client.tlsConfig)
			}
		})
	}
}

// Verifies that NewClient returns descriptive errors for invalid configs:
// missing CA file and malformed client certificate/key pair.
func TestNewClient_Error_Returns_Descriptive_Failure(t *testing.T) {
	t.Parallel()

	caCertPEM, _ := generateSelfSignedCert(t)
	caPath := writeTempFile(t, "ca-cert.pem", caCertPEM)

	table := []struct {
		name           string
		config         Config
		expectedErrMsg string
	}{
		{
			name:           "Nonexistent_CACertPath_Fails_With_File_Error",
			config:         Config{Host: "localhost:8443", CACertPath: "/nonexistent/path/ca.pem"},
			expectedErrMsg: "failed to open ca cert",
		},
		{
			name: "Invalid_Client_Certificate_Fails_With_KeyPair_Error",
			config: Config{
				Host:        "localhost:8443",
				CACertPath:  caPath,
				TLSCertPath: writeTempFile(t, "bad-cert.pem", []byte("not-a-cert")),
				TLSKeyPath:  writeTempFile(t, "bad-key.pem", []byte("not-a-key")),
			},
			expectedErrMsg: "failed to load x509 key pair",
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(&tc.config)

			require.Error(t, err)
			require.Nil(t, client)
			require.Contains(t, err.Error(), tc.expectedErrMsg)
		})
	}
}

// --- CallView / CallViewWithContext ---

// Verifies the full happy path: CallView sends a PUT to /v1/Views/{fid},
// deserializes the JSON-encoded CallViewResponse, and returns the Result.
func TestCallView_Returns_Result_On_Valid_Server_Response(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		require.Equal(t, "/v1/Views/myView", r.URL.Path)

		resp := &protos.CommandResponse_CallViewResponse{
			CallViewResponse: &protos.CallViewResponse{
				Result: []byte("hello-world"),
			},
		}
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	result, err := client.CallView("myView", []byte(`{"key":"value"}`))
	require.NoError(t, err)
	require.NotNil(t, result)
}

// Verifies that CallView surfaces clear errors for each server-side failure mode:
// HTTP error status, unparseable body, and a response with a nil inner message.
func TestCallView_Error_Returns_Descriptive_Failure(t *testing.T) {
	t.Parallel()

	table := []struct {
		name           string
		handler        http.HandlerFunc
		expectedErrMsg string
	}{
		{
			name: "Non_200_Status_Code_Returns_Status_Error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedErrMsg: "status code",
		},
		{
			name: "Invalid_JSON_Body_Returns_Unmarshal_Error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not-json"))
			},
			expectedErrMsg: "failed to unmarshal response",
		},
		{
			name: "Null_CallViewResponse_Returns_Invalid_Response_Error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				resp := &protos.CommandResponse_CallViewResponse{}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(resp)
			},
			expectedErrMsg: "invalid response",
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(tc.handler)
			defer server.Close()

			client := newTestClient(t, server)

			result, err := client.CallView("someView", nil)
			require.Error(t, err)
			require.Nil(t, result)
			require.Contains(t, err.Error(), tc.expectedErrMsg)
		})
	}
}

// Verifies that CallViewWithContext propagates context cancellation
// and does not hang when the caller cancels before the server responds.
func TestCallViewWithContext_Cancelled_Context_Returns_Error(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newTestClient(t, server)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := client.CallViewWithContext(ctx, "slowView", nil)
	require.Error(t, err)
	require.Nil(t, result)
}

// --- Metrics ---

// Verifies that Metrics() correctly parses a Prometheus text response into
// MetricFamily objects on success, and returns a wrapped error on HTTP failure.
func TestMetrics_Parses_And_Returns_Prometheus_Families(t *testing.T) {
	t.Parallel()

	table := []struct {
		name           string
		statusCode     int
		body           string
		expectErr      bool
		expectedErrMsg string
		expectedFamily string
	}{
		{
			name:           "Valid_Prometheus_Response_Returns_Parsed_Families",
			statusCode:     http.StatusOK,
			body:           "# HELP go_goroutines Number of goroutines.\n# TYPE go_goroutines gauge\ngo_goroutines 42\n",
			expectedFamily: "go_goroutines",
		},
		{
			name:           "Server_Error_Returns_Wrapped_Failure",
			statusCode:     http.StatusInternalServerError,
			expectErr:      true,
			expectedErrMsg: "failed calling metrics",
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/metrics", r.URL.Path)

				w.WriteHeader(tc.statusCode)
				if tc.body != "" {
					_, _ = w.Write([]byte(tc.body))
				}
			}))
			defer server.Close()

			client := newTestClient(t, server)
			families, err := client.Metrics()

			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, families)
				require.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, families)
				require.Contains(t, families, tc.expectedFamily)
			}
		})
	}
}

// --- Helpers ---

// newTestClient bypasses NewClient to avoid real TLS; points directly at an httptest.Server.
func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	return &Client{
		c:             server.Client(),
		url:           server.URL,
		wsUrl:         "ws" + server.URL[4:],
		metricsParser: expfmt.NewTextParser(model.LegacyValidation),
	}
}

// generateSelfSignedCert returns a PEM-encoded self-signed CA cert and private key.
func generateSelfSignedCert(t *testing.T) (certPEM []byte, keyPEM []byte) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Test"}},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path
}
