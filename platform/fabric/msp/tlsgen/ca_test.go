/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsgen

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
)

func createTLSService(t *testing.T, ca CA, host string) *grpc.Server {
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
		require.NoError(t, err)
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
	tlsCA, err := NewCA()
	require.NoError(t, err)
	require.Equal(t, tlsCA.(*ca).caCert.Signer, tlsCA.Signer())
}
