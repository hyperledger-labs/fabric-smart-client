/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/stretchr/testify/assert"
)

func TestCreds(t *testing.T) {
	t.Parallel()

	caPEM, err := os.ReadFile(filepath.Join("testdata", "certs", "Org1-cert.pem"))
	if err != nil {
		t.Fatalf("failed to read root certificate: %v", err)
	}
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(caPEM)
	if !ok {
		t.Fatalf("failed to create certPool")
	}
	cert, err := tls.LoadX509KeyPair(
		filepath.Join("testdata", "certs", "Org1-server1-cert.pem"),
		filepath.Join("testdata", "certs", "Org1-server1-key.pem"),
	)
	if err != nil {
		t.Fatalf("failed to load TLS certificate [%s]", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	config := grpc.NewTLSConfig(tlsConfig)

	logger, recorder := logging.NewTestLogger(t)

	creds := grpc.NewServerTransportCredentials(config, logger)
	_, _, err = creds.ClientHandshake(context.Background(), "", nil)
	assert.EqualError(t, err, grpc.ErrClientHandshakeNotImplemented.Error())
	//lint:ignore SA1019: creds.OverrideServerName is deprecated: use grpc.WithAuthority instead. Will be supported throughout 1.x.
	err = creds.OverrideServerName("") //nolint:all
	assert.EqualError(t, err, grpc.ErrOverrideHostnameNotSupported.Error())
	//lint:ignore SA1019: creds.Info().SecurityVersion is deprecated: please use Peer.AuthInfo.
	assert.Equal(t, "1.2", creds.Info().SecurityVersion) //nolint:all
	assert.Equal(t, "tls", creds.Info().SecurityProtocol)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to start listener [%s]", err)
	}
	defer lis.Close()

	_, port, err := net.SplitHostPort(lis.Addr().String())
	assert.NoError(t, err)
	addr := net.JoinHostPort("localhost", port)

	handshake := func(wg *sync.WaitGroup) {
		defer wg.Done()
		conn, err := lis.Accept()
		if err != nil {
			t.Logf("failed to accept connection [%s]", err)
		}
		_, _, err = creds.ServerHandshake(conn)
		if err != nil {
			t.Logf("ServerHandshake error [%s]", err)
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go handshake(wg)
	_, err = tls.Dial("tcp", addr, &tls.Config{RootCAs: certPool})
	wg.Wait()
	assert.NoError(t, err)

	wg = &sync.WaitGroup{}
	wg.Add(1)
	go handshake(wg)
	_, err = tls.Dial("tcp", addr, &tls.Config{
		RootCAs:    certPool,
		MaxVersion: tls.VersionTLS10,
	})
	wg.Wait()
	assert.True(t,
		strings.Contains(err.Error(), "tls: no supported versions satisfy MinVersion and MaxVersion") || // go1.17
			strings.Contains(err.Error(), "protocol version not supported"), // go1.18
	)
	assert.Contains(t, recorder.Messages()[0], "TLS handshake failed with error")
}

func TestNewTLSConfig(t *testing.T) {
	t.Parallel()
	tlsConfig := &tls.Config{}

	config := grpc.NewTLSConfig(tlsConfig)

	assert.NotEmpty(t, config, "TLSConfig is not empty")
}

func TestConfig(t *testing.T) {
	t.Parallel()
	config := grpc.NewTLSConfig(&tls.Config{
		ServerName: "bueno",
	})

	configCopy := config.Config()

	certPool := x509.NewCertPool()
	config.SetClientCAs(certPool)

	assert.NotEqual(t, config.Config(), &configCopy, "TLSConfig should have new certs")
}

func TestAddRootCA(t *testing.T) {
	t.Parallel()

	caPEM, err := os.ReadFile(filepath.Join("testdata", "certs", "Org1-cert.pem"))
	if err != nil {
		t.Fatalf("failed to read root certificate: %v", err)
	}

	cert := &x509.Certificate{
		EmailAddresses: []string{"test@foobar.com"},
	}

	expectedCertPool := x509.NewCertPool()
	ok := expectedCertPool.AppendCertsFromPEM(caPEM)
	if !ok {
		t.Fatalf("failed to create expected certPool")
	}

	expectedCertPool.AddCert(cert)

	certPool := x509.NewCertPool()
	ok = certPool.AppendCertsFromPEM(caPEM)
	if !ok {
		t.Fatalf("failed to create certPool")
	}

	tlsConfig := &tls.Config{
		ClientCAs: certPool,
	}
	config := grpc.NewTLSConfig(tlsConfig)

	assert.Equal(t, config.Config().ClientCAs, certPool)

	config.AddClientRootCA(cert)

	//lint:ignore SA1019: config.Config().ClientCAs.Subjects has been deprecated since Go 1.18: if s was returned by [SystemCertPool], Subjects will not include the system roots.
	assert.Equal(t, config.Config().ClientCAs.Subjects(), expectedCertPool.Subjects(), "The CertPools should be equal") //nolint:all
}

func TestSetClientCAs(t *testing.T) {
	t.Parallel()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{},
	}
	config := grpc.NewTLSConfig(tlsConfig)

	assert.Empty(t, config.Config().ClientCAs, "No CertPool should be defined")

	certPool := x509.NewCertPool()
	config.SetClientCAs(certPool)

	assert.NotNil(t, config.Config().ClientCAs, "The CertPools' should not be the same")
}
