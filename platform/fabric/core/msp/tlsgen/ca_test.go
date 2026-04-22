/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsgen

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

func createTLSService(t *testing.T, ca CA, host string) *grpc.Server {
	t.Helper()
	keyPair, err := ca.NewServerCertKeyPair(host)
	require.NoError(t, err)
	cert, err := tls.X509KeyPair(keyPair.Cert, keyPair.Key)
	require.NoError(t, err)
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    x509.NewCertPool(),
	}
	tlsConf.ClientCAs.AppendCertsFromPEM(ca.CertBytes())
	return grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsConf)))
}

func TestTLSCA(t *testing.T) {
	t.Parallel()
	// This test checks that the CA can create certificates
	// and corresponding keys that are signed by itself

	ca, err := NewCA()
	require.NoError(t, err)
	require.NotNil(t, ca)

	srv := createTLSService(t, ca, "127.0.0.1")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		err = srv.Serve(listener)
		if errors.Is(err, grpc.ErrServerStopped) {
			return
		}
		assert.NoError(t, err)
	}()
	defer func() {
		srv.Stop()
		utils.IgnoreError(listener.Close())
	}()

	probeTLS := func(kp *CertKeyPair) error {
		cert, err := tls.X509KeyPair(kp.Cert, kp.Key)
		require.NoError(t, err)
		tlsCfg := &tls.Config{
			RootCAs:      x509.NewCertPool(),
			Certificates: []tls.Certificate{cert},
		}
		tlsCfg.RootCAs.AppendCertsFromPEM(ca.CertBytes())
		tlsOpts := grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))

		conn, err := grpc.NewClient(listener.Addr().String(), tlsOpts)
		if err != nil {
			return err
		}

		// let's connect
		conn.Connect()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		for {
			state := conn.GetState()
			if state == connectivity.Ready {
				break
			}
			if !conn.WaitForStateChange(ctx, state) {
				return errors.Wrapf(ctx.Err(), "gRPC connection not ready before timeout (state=%s)", state)
			}
		}
		return conn.Close()
	}

	// Good path - use a cert key pair generated from the CA
	// that the TLS server started with
	kp, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	err = probeTLS(kp)
	require.NoError(t, err)

	// Bad path - use a cert key pair generated from a foreign CA
	foreignCA, _ := NewCA()
	kp, err = foreignCA.NewClientCertKeyPair()
	require.NoError(t, err)
	err = probeTLS(kp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context deadline exceeded")
}

func TestTLSCASigner(t *testing.T) {
	t.Parallel()
	tlsCA, err := NewCA()
	require.NoError(t, err)
	require.Equal(t, tlsCA.(*ca).caCert.Signer, tlsCA.Signer())
}

func TestIntermediateCA(t *testing.T) {
	t.Parallel()

	// Create root CA
	rootCA, err := NewCA()
	require.NoError(t, err)
	require.NotNil(t, rootCA)

	// Create intermediate CA
	intermediateCA, err := rootCA.NewIntermediateCA()
	require.NoError(t, err)
	require.NotNil(t, intermediateCA)

	// Intermediate CA cert should differ from root CA cert
	require.NotEqual(t, rootCA.CertBytes(), intermediateCA.CertBytes())

	// Verify intermediate CA cert is signed by the root CA and is a CA
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootCA.CertBytes())

	block, _ := pem.Decode(intermediateCA.CertBytes())
	require.NotNil(t, block)
	intermediateCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.True(t, intermediateCert.IsCA)

	_, err = intermediateCert.Verify(x509.VerifyOptions{
		Roots: rootPool,
	})
	require.NoError(t, err)

	// Generate a client cert from the intermediate CA
	clientKP, err := intermediateCA.NewClientCertKeyPair()
	require.NoError(t, err)
	require.NotNil(t, clientKP)

	// Verify that the client cert is trusted by the root CA (with intermediate cert in between)
	clientBlock, _ := pem.Decode(clientKP.Cert)
	require.NotNil(t, clientBlock)
	clientCert, err := x509.ParseCertificate(clientBlock.Bytes)
	require.NoError(t, err)

	intermediatePool := x509.NewCertPool()
	intermediatePool.AppendCertsFromPEM(intermediateCA.CertBytes())

	_, err = clientCert.Verify(x509.VerifyOptions{
		Roots:         rootPool,
		Intermediates: intermediatePool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	require.NoError(t, err)
}

func TestNewServerCertKeyPair_MultipleHosts(t *testing.T) {
	t.Parallel()

	ca, err := NewCA()
	require.NoError(t, err)

	// Mix of IP addresses and DNS names
	hosts := []string{"127.0.0.1", "localhost", "10.0.0.1", "example.com"}
	kp, err := ca.NewServerCertKeyPair(hosts...)
	require.NoError(t, err)
	require.NotNil(t, kp)

	// Parse the generated certificate and verify host entries
	block, _ := pem.Decode(kp.Cert)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Verify IP addresses are correctly set
	require.Len(t, cert.IPAddresses, 2, "expected 2 IP addresses")
	var ips []string
	for _, ip := range cert.IPAddresses {
		ips = append(ips, ip.String())
	}
	require.Contains(t, ips, "127.0.0.1")
	require.Contains(t, ips, "10.0.0.1")

	// Verify DNS names are correctly set
	require.Len(t, cert.DNSNames, 2, "expected 2 DNS names")
	require.Contains(t, cert.DNSNames, "localhost")
	require.Contains(t, cert.DNSNames, "example.com")

	// Verify the cert is a server cert (has ServerAuth usage)
	require.Contains(t, cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
}
